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

// sealKeyToModeenv seals the supplied key to the parameters specified in modeenv
// during system install.
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
		return fmt.Errorf("cannot build recover mode load sequences: %v", err)
	}

	bl, err := bootloader.Find(InitramfsUbuntuBootDir, &bootloader.Options{
		NoSlashBoot: true,
	})
	if err != nil {
		return fmt.Errorf("cannot find the bootloader: %v", err)
	}

	runModeChains, err := runModeLoadSequences(rbl, bl, modeenv)
	if err != nil {
		return fmt.Errorf("cannot build run mode load sequences: %v", err)
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

// recoverModeLoadSequences builds the EFI binary load sequences for recover mode.
func recoverModeLoadSequences(rbl bootloader.Bootloader, modeenv *Modeenv) ([][]string, error) {
	tbl, ok := rbl.(bootloader.TrustedAssetsBootloader)
	if !ok {
		return nil, fmt.Errorf("bootloader doesn't support trusted assets")
	}

	bootChain, err := tbl.RecoveryBootChain(filepath.Join("systems", modeenv.RecoverySystem))
	if err != nil {
		return nil, err
	}

	seq0 := make([]string, 0, len(bootChain))
	seq1 := make([]string, 0, len(bootChain))

	// walk the chain and recover cache entries for the trusted assets
	for _, image := range bootChain {
		switch image.Type {
		case boot.RecoveryBootAsset:
			p0, p1, err = cachedAssetPathnames(rbl.Name(), image.Path(), modeenv.CurrentTrustedRecoveryBootAssets)
			if err != nil {
				return nil, err
			}
			seq0 = append(seq0, p0)
			seq1 = append(seq1, p1)
		case boot.RecoveryKernel:
			seq0 = append(seq0, image)
			seq0 = append(seq1, image)
		default:
			return nil, fmt.Errorf("invalid entry in the recovery boot chain")
		}
	}

	if sequenceEquals(seq0, seq1) {
		return [][]string{seq0}, nil
	}

	return [][]string{seq0, seq1}, nil

	/*
		seq0, seq1, err := loadSequencesForBootloader(rbl, modeenv.CurrentTrustedRecoveryBootAssets)
		if err != nil {
			return nil, err
		}

		// set a single kernel path because we don't support updating the recovery system yet
		kernel, err := recoverModeKernelFromModeenv(rbl, modeenv)
		if err != nil {
			return nil, err
		}

		seq0 = append(seq0, kernel)
		seq1 = append(seq1, kernel)

		if listEquals(seq0, seq1) {
			return [][]string{seq0}, nil
		}

		return [][]string{seq0, seq1}, nil
	*/
}

// runModeLoadSequences builds the EFI binary load sequences for run mode.
func runModeLoadSequences(rbl, bl bootloader.Bootloader, modeenv *Modeenv) ([][]string, error) {
	tbl, ok := rbl.(bootloader.TrustedAssetsBootloader)
	if !ok {
		return nil, fmt.Errorf("bootloader doesn't support trusted assets")
	}

	bootChain, err := tbl.BootChain(bl)
	if err != nil {
		return nil, err
	}

	seq0 := make([]string, 0, len(bootChain))
	seq1 := make([]string, 0, len(bootChain))

	// walk the chain and recover cache entries for the trusted assets
	for _, image := range bootChain {
		switch image.Type {
		case boot.RecoveryBootAsset:
			p0, p1, err = cachedAssetPathnames(rbl.Name(), image.Path(), modeenv.CurrentTrustedRecoveryBootAssets)
			if err != nil {
				return nil, err
			}
			seq0 = append(seq0, p0)
			seq1 = append(seq1, p1)
		case boot.BootAsset:
			p0, p1, err = cachedAssetPathnames(bl.Name(), image.Path(), modeenv.CurrentTrustedBootAssets)
			if err != nil {
				return nil, err
			}
			seq0 = append(seq0, p0)
			seq1 = append(seq1, p1)
		case boot.BootKernel:
			current, next, err := runModeKernelsFromModeenv(modeenv)
			if err != nil {
				return nil, err
			}
			seq0 = append(seq0, current)
			seq1 = append(seq1, next)
		default:
			return nil, fmt.Errorf("invalid entry in the boot chain")
		}
	}

	if sequenceEquals(seq0, seq1) {
		return [][]string{seq0}, nil
	}

	return [][]string{seq0, seq1}, nil

	/*
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

		current, next, err := runModeKernelsFromModeenv(modeenv)
		if err != nil {
			return nil, err
		}
		seq0 = append(seq0, current)
		seq1 = append(seq1, next)

		if listEquals(seq0, seq1) {
			return [][]string{seq0}, nil
		}

		return [][]string{seq0, seq1}, nil
	*/
}

// runModeKernelsFromModeenv obtains the current and next kernels listed in modeenv.
func runModeKernelsFromModeenv(modeenv *Modeenv) (string, string, error) {
	switch len(modeenv.CurrentKernels) {
	case 1:
		current := filepath.Join(dirs.SnapBlobDir, modeenv.CurrentKernels[0])
		return current, current, nil
	case 2:
		current := filepath.Join(dirs.SnapBlobDir, modeenv.CurrentKernels[0])
		next := filepath.Join(dirs.SnapBlobDir, modeenv.CurrentKernels[1])
		return current, next, nil
	}
	return "", "", fmt.Errorf("invalid number of kernels in modeenv")
}

// cachedAssetPathnames returns the pathnames of the files corresponding to the current
// and next instances of a given boot asset.
func cachedAssetPathnames(blName, name string, assetsMap bootAssetsMap) (current, next string, err error) {
	cacheEntry := func(hash string) string {
		return filepath.Join(dirs.SnapBootAssetsDir, blName, fmt.Sprintf("%s-%s", name, hash))
	}

	hashList, ok := assetsMap[name]
	if !ok {
		return "", "", fmt.Errorf("cannot find asset %s in modeenv", name)
	}

	switch len(hashList) {
	case 1:
		current = cacheEntry(hashList[0])
		next = current
	case 2:
		current = cacheEntry(hashList[0])
		next = cacheEntry(hashList[1])
	default:
		return "", "", fmt.Errorf("invalid number of hashes for asset %s in modeenv", name)
	}
	return current, next, nil
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
