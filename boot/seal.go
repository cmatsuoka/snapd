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
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/secboot"
)

func sealKeyToModeenv(key secboot.EncryptionKey, blName string, model *asserts.Model, modeenv *Modeenv) error {
	// Build the recover mode load sequences
	recoverModeChains, err := recoverModeLoadSequences(blName, modeenv)
	if err != nil {
		return fmt.Errorf("cannot determine recover mode load sequences: %v", err)
	}
	runModeChains, err := runModeLoadSequences(blName, modeenv)
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

	sealKeyParams := secboot.SealKeyParams{
		ModelParams: []*secboot.SealKeyModelParams{
			{
				Model:          model,
				KernelCmdlines: kernelCmdlines,
				EFILoadChains:  append(runModeChains, recoverModeChains...),
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

func recoverModeLoadSequences(blName string, modeenv *Modeenv) ([][]string, error) {
	kernelPath, err := kernelPathFromModeenv(modeenv)
	if err != nil {
		return nil, err
	}

	// recover mode load chains have the shim, the recovery partition bootloader, and
	// the recovery system kernel snap.
	seq0 := make([]string, 3)
	seq1 := make([]string, 3)

	seq0[0], seq1[0], err = cachedAssetPathnames(blName, "bootx64.efi", modeenv.CurrentTrustedRecoveryBootAssets)
	if err != nil {
		return nil, err
	}
	seq0[1], seq1[1], err = cachedAssetPathnames(blName, "grubx64.efi", modeenv.CurrentTrustedRecoveryBootAssets)
	if err != nil {
		return nil, err
	}
	seq0[2] = kernelPath
	seq1[2] = kernelPath

	if listEquals(seq0, seq1) {
		return [][]string{seq0}, nil
	}

	return [][]string{seq0, seq1}, nil
}

func runModeLoadSequences(blName string, modeenv *Modeenv) ([][]string, error) {
	// run mode load chains have the shim, the recovery partition bootloader, the boot
	// partition bootloader, and the extracted kernel file.
	seq0 := make([]string, 4)
	seq1 := make([]string, 4)

	var err error
	seq0[0], seq1[0], err = cachedAssetPathnames(blName, "bootx64.efi", modeenv.CurrentTrustedRecoveryBootAssets)
	if err != nil {
		return nil, err
	}
	seq0[1], seq1[1], err = cachedAssetPathnames(blName, "grubx64.efi", modeenv.CurrentTrustedRecoveryBootAssets)
	if err != nil {
		return nil, err
	}
	seq0[3], seq1[3], err = cachedAssetPathnames(blName, "grubx64.efi", modeenv.CurrentTrustedBootAssets)
	if err != nil {
		return nil, err
	}
	// XXX: determine the correct kernel paths
	seq0[2] = filepath.Join(InitramfsRunMntDir, "ubuntu-boot/EFI/ubuntu/kernel.efi")
	seq1[2] = seq0[2]

	if listEquals(seq0, seq1) {
		return [][]string{seq0}, nil
	}

	return [][]string{seq0, seq1}, nil
}

func kernelPathFromModeenv(modeenv *Modeenv) (string, error) {
	// XXX: using the extracted kernel
	return filepath.Join(InitramfsRunMntDir, "ubuntu-boot/EFI/ubuntu/kernel.efi"), nil

	if len(modeenv.CurrentKernels) < 1 {
		return "", fmt.Errorf("cannot determine kernel path")
	}
	kernelPath := filepath.Join(InitramfsUbuntuSeedDir, "systems", modeenv.RecoverySystem, "snaps", modeenv.CurrentKernels[0])
	logger.Debugf("trying kernel path: %s", kernelPath)
	if osutil.FileExists(kernelPath) {
		return kernelPath, nil
	}

	kernelPath = filepath.Join(InitramfsUbuntuSeedDir, "snaps", modeenv.CurrentKernels[0])
	logger.Debugf("trying kernel path: %s", kernelPath)
	if osutil.FileExists(kernelPath) {
		return kernelPath, nil
	}

	return "", fmt.Errorf("kernel file not found")
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
