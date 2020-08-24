// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2020 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package boot

import (
	"fmt"
	"path/filepath"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/bootloader"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/secboot"
)

func sealKeyToModeenv(key secboot.EncryptionKey, model *asserts.Model, modeenv *Modeenv) error {
	// Build the recover mode load sequences
	rbl, err := bootloader.Find(InitramfsUbuntuSeedDir, &bootloader.Options{
		NoSlashBoot: true,
		Recovery:    true,
	})
	if err != nil {
		return fmt.Errorf("cannot find the recovery bootloader: %v", err)
	}

	recoverModeChains, err := recoverModeLoadSequences(rbl, modeenv)
	if err != nil {
		return fmt.Errorf("cannot determine recover mode load sequences: %v", err)
	}

	bl, err := bootloader.Find(InitramfsUbuntuBootDir, &bootloader.Options{
		NoSlashBoot: true,
	})
	if err != nil {
		return fmt.Errorf("cannot find the bootloader: %v", err)
	}

	runModeChains, err := runModeLoadSequences(rbl, bl, modeenv)
	if err != nil {
		return fmt.Errorf("cannot determine run mode load sequences: %v", err)
	}

	// TODO:UC20: retrieve command lines from modeenv, the format is still TBD
	// Get the expected kernel command line for the system that is currently being installed
	cmdline, err := ComposeCandidateCommandLine(model)
	if err != nil {
		return fmt.Errorf("cannot obtain kernel command line: %v", err)
	}
	// Get the expected kernel command line of the recovery system we're installing from
	recoveryCmdline, err := ComposeRecoveryCommandLine(model, modeenv.RecoverySystem)
	if err != nil {
		return fmt.Errorf("cannot obtain recovery kernel command line: %v", err)
	}
	kernelCmdlines := []string{
		cmdline,
		recoveryCmdline,
	}

	_ = recoverModeChains
	sealKeyParams := secboot.SealKeyParams{
		ModelParams: []*secboot.SealKeyModelParams{
			{
				Model:          model,
				KernelCmdlines: kernelCmdlines,
				//EFILoadChains:  append(runModeChains, recoverModeChains...),
				EFILoadChains: runModeChains,
			},
		},
		KeyFile:                 filepath.Join(InitramfsEncryptionKeyDir, "ubuntu-data.sealed-key"),
		TPMPolicyUpdateDataFile: filepath.Join(InstallHostFDEDataDir, "policy-update-data"),
		TPMLockoutAuthFile:      filepath.Join(InstallHostFDEDataDir, "tpm-lockout-auth"),
	}

	if err := secbootSealKey(key, &sealKeyParams); err != nil {
		return fmt.Errorf("cannot seal the encryption key: %v", err)
	}
	return nil
}

// recoverModeLoadSequences builds the load sequences for recover mode.
func recoverModeLoadSequences(rbl bootloader.Bootloader, modeenv *Modeenv) ([][]string, error) {
	seq0, seq1, err := loadSequencesForBootloader(rbl, modeenv.CurrentTrustedRecoveryBootAssets)
	if err != nil {
		return nil, err
	}

	// set a single kernel path because we don't support updating the recovery system yet
	kernel, err := recoverModeKernelFromModeenv(modeenv)
	if err != nil {
		return nil, fmt.Errorf("cannot build load sequences for recover mode: %v", err)
	}

	seq0 = append(seq0, kernel)
	seq1 = append(seq1, kernel)

	if listEquals(seq0, seq1) {
		return [][]string{seq0}, nil
	}

	return [][]string{seq0, seq1}, nil
}

// runModeLoadSequences builds the load sequences for run mode.
func runModeLoadSequences(rbl, bl bootloader.Bootloader, modeenv *Modeenv) ([][]string, error) {
	recSeq0, recSeq1, err := loadSequencesForBootloader(rbl, modeenv.CurrentTrustedRecoveryBootAssets)
	if err != nil {
		return nil, err
	}

	runSeq0, runSeq1, err := loadSequencesForBootloader(bl, modeenv.CurrentTrustedBootAssets)
	if err != nil {
		return nil, err
	}

	seq0 := append(recSeq0, runSeq0...)
	seq1 := append(recSeq1, runSeq1...)

	kernels, err := runModeKernelsFromModeenv(modeenv)
	if err != nil {
		return nil, fmt.Errorf("cannot build load sequences for run mode: %v", err)
	}
	seq0 = append(seq0, kernels[0])
	seq1 = append(seq1, kernels[1])

	if listEquals(seq0, seq1) {
		return [][]string{seq0}, nil
	}

	return [][]string{seq0, seq1}, nil
}

func loadSequencesForBootloader(bl bootloader.Bootloader, assetsMap bootAssetsMap) (seq0, seq1 []string, err error) {
	assetNames, err := trustedAssetNamesForBootloader(bl)
	if err != nil {
		return seq0, seq1, err
	}
	num := len(assetNames)
	if num == 0 {
		return seq0, seq1, nil
	}

	seq0 = make([]string, num)
	seq1 = make([]string, num)

	for i, name := range assetNames {
		var err error
		seq0[i], seq1[i], err = cachedAssetPathnames(bl.Name(), name, assetsMap)
		if err != nil {
			return seq0, seq1, err
		}
	}

	return seq0, seq1, nil
}

func trustedAssetNamesForBootloader(bl bootloader.Bootloader) ([]string, error) {
	tbl, ok := bl.(bootloader.TrustedAssetsBootloader)
	if !ok {
		return nil, fmt.Errorf("bootloader doesn't support trusted assets")
	}
	assets, err := tbl.TrustedAssets()
	if err != nil {
		return nil, err
	}
	assetNames := make([]string, len(assets))
	for i, asset := range assets {
		assetNames[i] = filepath.Base(asset)
	}
	return assetNames, nil
}

func recoverModeKernelFromModeenv(modeenv *Modeenv) (string, error) {
	// XXX: determine the correct kernel paths
	kernelPath := filepath.Join(InitramfsRunMntDir, "ubuntu-boot/EFI/ubuntu/kernel.efi")
	return kernelPath, nil
}

func runModeKernelsFromModeenv(modeenv *Modeenv) ([]string, error) {
	switch len(modeenv.CurrentKernels) {
	case 1:
		current := filepath.Join(dirs.SnapBlobDir, modeenv.CurrentKernels[0])
		return []string{current, current}, nil
	case 2:
		current := filepath.Join(dirs.SnapBlobDir, modeenv.CurrentKernels[0])
		next := filepath.Join(dirs.SnapBlobDir, modeenv.CurrentKernels[1])
		return []string{current, next}, nil
	}
	return nil, fmt.Errorf("cannot determine run mode kernel")
}

func cachedAssetPathnames(blName, name string, assetsMap bootAssetsMap) (before, after string, err error) {
	cacheEntry := func(hash string) string {
		return filepath.Join(dirs.SnapBootAssetsDir, blName, fmt.Sprintf("%s-%s", name, hash))
	}

	hashList, ok := assetsMap[name]
	if !ok {
		return "", "", fmt.Errorf("cannot find a hash list for asset %s", name)
	}

	switch len(hashList) {
	case 1:
		before = cacheEntry(hashList[0])
		after = before
	case 2:
		before = cacheEntry(hashList[0])
		after = cacheEntry(hashList[1])
	default:
		return "", "", fmt.Errorf("invalid number of hashes for asset %s", name)
	}
	return before, after, nil
}

func listEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
