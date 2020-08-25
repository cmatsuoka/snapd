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

package boot_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/boot"
	"github.com/snapcore/snapd/bootloader"
	"github.com/snapcore/snapd/bootloader/bootloadertest"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/secboot"
	"github.com/snapcore/snapd/testutil"
)

type sealSuite struct {
	testutil.BaseTest
}

var _ = Suite(&sealSuite{})

func (s *sealSuite) TestCachedAssetPathnames(c *C) {
	assetsMap := boot.BootAssetsMap{
		"foo":  []string{"foo-hash-1"},
		"bar":  []string{"bar-hash-1", "bar-hash-2"},
		"baz":  []string{"baz-hash-1", "baz-hash-2", "baz-hash-3"},
		"quux": []string{},
	}

	p1, p2, err := boot.CachedAssetPathnames("bootloader", "foo", assetsMap)
	c.Assert(err, IsNil)
	c.Assert(p1, Equals, "/var/lib/snapd/boot-assets/bootloader/foo-foo-hash-1")
	c.Assert(p2, Equals, "/var/lib/snapd/boot-assets/bootloader/foo-foo-hash-1")

	p1, p2, err = boot.CachedAssetPathnames("bootloader", "bar", assetsMap)
	c.Assert(err, IsNil)
	c.Assert(p1, Equals, "/var/lib/snapd/boot-assets/bootloader/bar-bar-hash-1")
	c.Assert(p2, Equals, "/var/lib/snapd/boot-assets/bootloader/bar-bar-hash-2")

	p1, p2, err = boot.CachedAssetPathnames("bootloader", "baz", assetsMap)
	c.Assert(err, ErrorMatches, "invalid number of hashes for asset baz in modeenv")

	p1, p2, err = boot.CachedAssetPathnames("bootloader", "quux", assetsMap)
	c.Assert(err, ErrorMatches, "invalid number of hashes for asset quux in modeenv")

	p1, p2, err = boot.CachedAssetPathnames("bootloader", "quuux", assetsMap)
	c.Assert(err, ErrorMatches, "cannot find asset quuux in modeenv")
}

func (s *sealSuite) TestRunModeKernelsFromModeenv(c *C) {
	for _, tc := range []struct {
		kernels []string
		k1      string
		k2      string
		err     string
	}{
		{
			kernels: []string{"pc-kernel_1.snap"},
			k1:      "/var/lib/snapd/snaps/pc-kernel_1.snap",
			k2:      "/var/lib/snapd/snaps/pc-kernel_1.snap",
		},
		{
			kernels: []string{"pc-kernel_1.snap", "pc-kernel_2.snap"},
			k1:      "/var/lib/snapd/snaps/pc-kernel_1.snap",
			k2:      "/var/lib/snapd/snaps/pc-kernel_2.snap",
		},
		{
			kernels: []string{"pc-kernel_1.snap", "pc-kernel_2.snap", "pc-kernel_3.snap"},
			err:     "invalid number of kernels in modeenv",
		},
		{
			kernels: []string{},
			err:     "invalid number of kernels in modeenv",
		},
	} {
		modeenv := &boot.Modeenv{CurrentKernels: tc.kernels}
		k1, k2, err := boot.RunModeKernelsFromModeenv(modeenv)
		if tc.err == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, tc.err)
			continue
		}
		c.Assert(k1, Equals, tc.k1)
		c.Assert(k2, Equals, tc.k2)
	}
}

func (s *sealSuite) TestRecoverModeKernelFromModeenv(c *C) {
	for _, tc := range []struct {
		recovery string
		err      string
	}{
		{
			// happy case
			recovery: "20200825",
		},
		{
			// invalid recovery system
			recovery: "0",
			err:      `cannot determine kernel for recovery system "0"`,
		},
		{
			// unspecified recovery system
			recovery: "",
			err:      "recovery system is not defined in modeenv",
		},
	} {
		tmpDir := c.MkDir()
		modeenv := &boot.Modeenv{RecoverySystem: tc.recovery}

		err := createMockRecoverySystem(tmpDir, "20200825", "/snaps/pc-kernel_1.snap")
		c.Assert(err, IsNil)

		err = createMockGrubCfg(filepath.Join(tmpDir))
		c.Assert(err, IsNil)

		bl, err := bootloader.Find(tmpDir, &bootloader.Options{NoSlashBoot: true, Recovery: true})
		c.Assert(err, IsNil)

		k, err := boot.RecoverModeKernelFromModeenv(bl, modeenv)
		if tc.err == "" {
			c.Assert(err, IsNil)
			c.Assert(k, Equals, "/var/lib/snapd/seed/snaps/pc-kernel_1.snap")
		} else {
			c.Assert(err, ErrorMatches, tc.err)
		}
	}
}

func (s *sealSuite) TestRecoverModeKernelFromModeenvBadBootloader(c *C) {
	modeenv := &boot.Modeenv{}

	// set a non recovery-aware mock bootloader
	b := &bootloadertest.MockBootloader{}
	bootloader.Force(b)
	defer bootloader.Force(nil)

	bl, err := bootloader.Find("", nil)
	c.Assert(err, IsNil)

	_, err = boot.RecoverModeKernelFromModeenv(bl, modeenv)
	c.Assert(err, ErrorMatches, "bootloader is not recovery aware")
}

func (s *sealSuite) TestTrustedAssetNamesForBootloader(c *C) {
	// set up a trusted assets bootloader
	b1 := (&bootloadertest.MockBootloader{}).WithTrustedAssets()
	b1.TrustedAssetsList = []string{"/path/name/to/trusted-asset-1", "/path/name/to/trusted-asset-2"}
	bootloader.Force(b1)
	defer bootloader.Force(nil)

	bl, err := bootloader.Find("", nil)
	c.Assert(err, IsNil)

	assets, err := boot.TrustedAssetNamesForBootloader(bl)
	c.Assert(err, IsNil)
	c.Assert(assets, DeepEquals, []string{"trusted-asset-1", "trusted-asset-2"})

	// set a bootloader that doesn't manage boot assets
	var b2 bootloadertest.MockBootloader
	bootloader.Force(&b2)

	bl, err = bootloader.Find("", nil)
	c.Assert(err, IsNil)

	_, err = boot.TrustedAssetNamesForBootloader(bl)
	c.Assert(err, ErrorMatches, "bootloader doesn't support trusted assets")
}

func (s *sealSuite) TestLoadSequencesForBootloader(c *C) {
	for _, tc := range []struct {
		taList     []string
		expectedS1 []string
		expectedS2 []string
		err        string
	}{
		{
			// happy case with assets with one hash
			taList:     []string{"/some/path/ta0", "/some/other/path/ta1"},
			expectedS1: []string{"/var/lib/snapd/boot-assets/ta0-ta0-hash-1", "/var/lib/snapd/boot-assets/ta1-ta1-hash-1"},
			expectedS2: []string{"/var/lib/snapd/boot-assets/ta0-ta0-hash-1", "/var/lib/snapd/boot-assets/ta1-ta1-hash-1"},
		},
		{
			// happy case with an asset with two hashes
			taList:     []string{"/some/path/ta1", "/some/other/path/ta2"},
			expectedS1: []string{"/var/lib/snapd/boot-assets/ta1-ta1-hash-1", "/var/lib/snapd/boot-assets/ta2-ta2-hash-1"},
			expectedS2: []string{"/var/lib/snapd/boot-assets/ta1-ta1-hash-1", "/var/lib/snapd/boot-assets/ta2-ta2-hash-2"},
		},
		{
			// an asset has more than two hashes
			taList: []string{"/some/path/ta2", "/some/other/path/ta3"},
			err:    "invalid number of hashes for asset ta3 in modeenv",
		},
		{
			// a trusted asset has no hashes
			taList: []string{"/some/path/ta2", "/some/other/path/ta4"},
			err:    "invalid number of hashes for asset ta4 in modeenv",
		},
		{
			// a trusted asset is not listed in modeenv
			taList: []string{"/some/path/ta0", "/does/not/exist"},
			err:    "cannot find asset exist in modeenv",
		},
		{
			// no trusted assets
			taList:     []string{},
			expectedS1: []string{},
			expectedS2: []string{},
		},
	} {
		// set up a recovery-aware bootloader
		b1 := (&bootloadertest.MockBootloader{}).WithTrustedAssets()
		b1.TrustedAssetsList = tc.taList
		bootloader.Force(b1)
		defer bootloader.Force(nil)

		bl, err := bootloader.Find("", nil)
		c.Assert(err, IsNil)

		assetsMap := boot.BootAssetsMap{
			"ta4": []string{},
			"ta3": []string{"ta3-hash-1", "ta3-hash-2", "ta3-hash-3"},
			"ta2": []string{"ta2-hash-1", "ta2-hash-2"},
			"ta1": []string{"ta1-hash-1"},
			"ta0": []string{"ta0-hash-1"},
		}

		s1, s2, err := boot.LoadSequencesForBootloader(bl, assetsMap)
		if tc.err == "" {
			c.Assert(err, IsNil)
			c.Assert(s1, DeepEquals, tc.expectedS1)
			c.Assert(s2, DeepEquals, tc.expectedS2)
		} else {
			c.Assert(err, ErrorMatches, tc.err)
		}
	}
}

func (s *sealSuite) TestRecoverModeLoadSequences(c *C) {
	for _, tc := range []struct {
		assetsMap         boot.BootAssetsMap
		recoverySystem    string
		undefinedKernel   bool
		expectedSequences [][]string
		err               string
	}{
		{
			// transition sequences
			recoverySystem: "20200825",
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1", "grub-hash-2"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			expectedSequences: [][]string{
				{
					"/var/lib/snapd/boot-assets/grub/bootx64.efi-shim-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-grub-hash-1",
					"/var/lib/snapd/seed/snaps/pc-kernel_1.snap",
				},
				{
					"/var/lib/snapd/boot-assets/grub/bootx64.efi-shim-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-grub-hash-2",
					"/var/lib/snapd/seed/snaps/pc-kernel_1.snap",
				},
			},
		},
		{
			// non-transition sequence
			recoverySystem: "20200825",
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			expectedSequences: [][]string{
				{
					"/var/lib/snapd/boot-assets/grub/bootx64.efi-shim-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-grub-hash-1",
					"/var/lib/snapd/seed/snaps/pc-kernel_1.snap",
				},
			},
		},
		{
			// kernel not defined in grubenv
			undefinedKernel: true,
			recoverySystem:  "20200825",
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			err: `cannot determine kernel for recovery system "20200825"`,
		},
		{
			// unspecified recovery system
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			err: "recovery system is not defined in modeenv",
		},
		{
			// invalid recovery system
			recoverySystem: "0",
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			err: `cannot determine kernel for recovery system "0"`,
		},
	} {
		tmpDir := c.MkDir()

		var kernel string
		if !tc.undefinedKernel {
			kernel = "/snaps/pc-kernel_1.snap"
		}
		err := createMockRecoverySystem(tmpDir, "20200825", kernel)
		c.Assert(err, IsNil)

		err = createMockGrubCfg(tmpDir)
		c.Assert(err, IsNil)

		bl, err := bootloader.Find(tmpDir, &bootloader.Options{NoSlashBoot: true, Recovery: true})
		c.Assert(err, IsNil)

		modeenv := &boot.Modeenv{
			RecoverySystem:                   tc.recoverySystem,
			CurrentTrustedRecoveryBootAssets: tc.assetsMap,
		}

		sequences, err := boot.RecoverModeLoadSequences(bl, modeenv)
		if tc.err == "" {
			c.Assert(err, IsNil)
			c.Assert(sequences, DeepEquals, tc.expectedSequences)
		} else {
			c.Assert(err, ErrorMatches, tc.err)
		}
	}
}

func (s *sealSuite) TestRunModeLoadSequences(c *C) {
	for _, tc := range []struct {
		recoveryAssetsMap boot.BootAssetsMap
		assetsMap         boot.BootAssetsMap
		kernels           []string
		recoverySystem    string
		expectedSequences [][]string
		err               string
	}{
		{
			// transition sequences with new system bootloader
			recoverySystem: "20200825",
			recoveryAssetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"run-grub-hash-1", "run-grub-hash-2"},
			},
			kernels: []string{"pc-kernel_500.snap"},
			expectedSequences: [][]string{
				{
					"/var/lib/snapd/boot-assets/grub/bootx64.efi-shim-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-grub-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-run-grub-hash-1",
					"/var/lib/snapd/snaps/pc-kernel_500.snap",
				},
				{
					"/var/lib/snapd/boot-assets/grub/bootx64.efi-shim-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-grub-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-run-grub-hash-2",
					"/var/lib/snapd/snaps/pc-kernel_500.snap",
				},
			},
		},
		{
			// transition sequences with new kernel
			recoverySystem: "20200825",
			recoveryAssetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"run-grub-hash-1"},
			},
			kernels: []string{"pc-kernel_500.snap", "pc-kernel_501.snap"},
			expectedSequences: [][]string{
				{
					"/var/lib/snapd/boot-assets/grub/bootx64.efi-shim-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-grub-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-run-grub-hash-1",
					"/var/lib/snapd/snaps/pc-kernel_500.snap",
				},
				{
					"/var/lib/snapd/boot-assets/grub/bootx64.efi-shim-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-grub-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-run-grub-hash-1",
					"/var/lib/snapd/snaps/pc-kernel_501.snap",
				},
			},
		},
		{
			// no transition sequence
			recoverySystem: "20200825",
			recoveryAssetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"run-grub-hash-1"},
			},
			kernels: []string{"pc-kernel_500.snap"},
			expectedSequences: [][]string{
				{
					"/var/lib/snapd/boot-assets/grub/bootx64.efi-shim-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-grub-hash-1",
					"/var/lib/snapd/boot-assets/grub/grubx64.efi-run-grub-hash-1",
					"/var/lib/snapd/snaps/pc-kernel_500.snap",
				},
			},
		},
		{
			// no run mode assets
			recoverySystem: "20200825",
			recoveryAssetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			err: "cannot find asset grubx64.efi in modeenv",
		},
		{
			// no kernels listed in modeenv
			recoverySystem: "20200825",
			recoveryAssetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"grub-hash-1"},
				"bootx64.efi": []string{"shim-hash-1"},
			},
			assetsMap: boot.BootAssetsMap{
				"grubx64.efi": []string{"run-grub-hash-1"},
			},
			err: "invalid number of kernels in modeenv",
		},
	} {
		tmpDir := c.MkDir()

		err := createMockRecoverySystem(tmpDir, "20200825", "/snaps/pc-kernel_1.snap")
		c.Assert(err, IsNil)

		err = createMockGrubCfg(tmpDir)
		c.Assert(err, IsNil)

		rbl, err := bootloader.Find(tmpDir, &bootloader.Options{NoSlashBoot: true, Recovery: true})
		c.Assert(err, IsNil)

		bl, err := bootloader.Find(tmpDir, &bootloader.Options{NoSlashBoot: true})
		c.Assert(err, IsNil)

		modeenv := &boot.Modeenv{
			RecoverySystem:                   tc.recoverySystem,
			CurrentTrustedRecoveryBootAssets: tc.recoveryAssetsMap,
			CurrentTrustedBootAssets:         tc.assetsMap,
			CurrentKernels:                   tc.kernels,
		}

		sequences, err := boot.RunModeLoadSequences(rbl, bl, modeenv)
		if tc.err == "" {
			c.Assert(err, IsNil)
			c.Assert(sequences, DeepEquals, tc.expectedSequences)
		} else {
			c.Assert(err, ErrorMatches, tc.err)
		}
	}
}

func (s *sealSuite) TestSealKeyToModeenv(c *C) {
	tmpDir := c.MkDir()
	dirs.SetRootDir(tmpDir)
	defer dirs.SetRootDir("")

	err := createMockRecoverySystem(filepath.Join(tmpDir, "run/mnt/ubuntu-seed"), "20200825", "/snaps/pc-kernel_1.snap")
	c.Assert(err, IsNil)

	err = createMockGrubCfg(filepath.Join(tmpDir, "run/mnt/ubuntu-seed"))
	c.Assert(err, IsNil)

	err = createMockGrubCfg(filepath.Join(tmpDir, "run/mnt/ubuntu-boot"))
	c.Assert(err, IsNil)

	modeenv := &boot.Modeenv{
		RecoverySystem: "20200825",
		CurrentTrustedRecoveryBootAssets: boot.BootAssetsMap{
			"grubx64.efi": []string{"grub-hash-1"},
			"bootx64.efi": []string{"shim-hash-1"},
		},

		CurrentTrustedBootAssets: boot.BootAssetsMap{
			"grubx64.efi": []string{"run-grub-hash-1"},
		},

		CurrentKernels: []string{"pc-kernel_500.snap"},
	}

	// set encryption key
	myKey := secboot.EncryptionKey{}
	for i := range myKey {
		myKey[i] = byte(i)
	}

	model := makeMockUC20Model()

	// set mock key sealing
	sealKeyCalls := 0
	restore := boot.MockSecbootSealKey(func(key secboot.EncryptionKey, params *secboot.SealKeyParams) error {
		sealKeyCalls++
		c.Check(key, DeepEquals, myKey)
		c.Assert(params.ModelParams, HasLen, 1)
		c.Assert(params.ModelParams[0].Model.DisplayName(), Equals, "My Model")
		cachedir := filepath.Join(tmpDir, "var/lib/snapd/boot-assets/grub")
		c.Assert(params.ModelParams[0].EFILoadChains, DeepEquals, [][]string{
			// run mode load sequence
			{
				filepath.Join(cachedir, "bootx64.efi-shim-hash-1"),
				filepath.Join(cachedir, "grubx64.efi-grub-hash-1"),
				filepath.Join(cachedir, "grubx64.efi-run-grub-hash-1"),
				filepath.Join(tmpDir, "var/lib/snapd/snaps/pc-kernel_500.snap"),
			},
			// recover mode load sequence
			{
				filepath.Join(cachedir, "bootx64.efi-shim-hash-1"),
				filepath.Join(cachedir, "grubx64.efi-grub-hash-1"),
				filepath.Join(tmpDir, "var/lib/snapd/seed/snaps/pc-kernel_1.snap"),
			},
		})
		c.Assert(params.ModelParams[0].KernelCmdlines, DeepEquals, []string{
			"snapd_recovery_mode=run console=ttyS0 console=tty1 panic=-1",
			"snapd_recovery_mode=recover snapd_recovery_system=20200825 console=ttyS0 console=tty1 panic=-1",
		})
		return nil
	})
	defer restore()

	err = boot.SealKeyToModeenv(myKey, model, modeenv)
	c.Assert(err, IsNil)
	c.Assert(sealKeyCalls, Equals, 1)

}

func createMockGrubCfg(baseDir string) error {
	cfg := filepath.Join(baseDir, "EFI/ubuntu/grub.cfg")
	if err := os.MkdirAll(filepath.Dir(cfg), 0755); err != nil {
		return err
	}
	return ioutil.WriteFile(cfg, []byte("# Snapd-Boot-Config-Edition: 1\n"), 0644)
}

func createMockRecoverySystem(seedDir, sysLabel, kernel string) error {
	recoverySystemDir := filepath.Join(seedDir, "systems", sysLabel)
	if err := os.MkdirAll(recoverySystemDir, 0755); err != nil {
		return err
	}
	envData := make([]byte, 1024)
	copy(envData, []byte(fmt.Sprintf("# GRUB Environment Block\nsnapd_recovery_kernel=%v\n", kernel)))
	return ioutil.WriteFile(filepath.Join(recoverySystemDir, "grubenv"), envData, 0644)
}
