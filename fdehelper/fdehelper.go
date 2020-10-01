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

package fdehelper

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/bootloader"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
)

// Enabled returns whether the external FDE helper should
// be called.
func Enabled() bool {
	// XXX: verify if we should use it.
	return true
}

// Supported calls the external FDE helper to check if this
// system supports encryption.
func Supported(gadgetDir string) error {
	// this is used only during the installation process
	out, err := exec.Command(filepath.Join(gadgetDir, "fde-helper"), "--supported").CombinedOutput()
	if err != nil {
		return osutil.OutputErr(out, err)
	}
	logger.Noticef("FDE helper supported")
	return nil
}

type UnlockParams struct {
	VolumeName       string `json:"volume-name"`
	SourceDevicePath string `json:"source-device-path"`
	LockKeysOnFinish bool   `json:"lock-keys-on-finish"`
}

// Unlock unseals the key and unlocks the encrypted volume
// specified in params.
func Unlock(params *UnlockParams) error {
	j, err := json.Marshal(params)
	if err != nil {
		return err
	}

	cmd := exec.Command("/run/mnt/ubuntu-boot/fde-helper", "--unlock")
	cmd.Stdin = bytes.NewReader(j)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return osutil.OutputErr(out, err)
	}
	return nil
}

// XXX: structures LoadChain and ModelParams are similar to
//      existing internal structures and are used to interface
//      with the external helper.

type LoadChain struct {
	Path string          `json:"path"`
	Snap string          `json:"snap,omitempty"`
	Role bootloader.Role `json:"role"`
	Next []*LoadChain    `json:"next,omitempty"`
}

type ModelParams struct {
	Series         string             `json:"series"`
	BrandID        string             `json:"brand-id"`
	Model          string             `json:"model"`
	Grade          asserts.ModelGrade `json:"grade"`
	SignKeyID      string             `json:"sign-key-id"`
	LoadChains     []*LoadChain       `json:"load-chains"`
	KernelCmdlines []string           `json:"kernel-cmdlines"`
}

// XXX: what should be the parameters sent to the initial
//      provision helper? If sending just boot chains we'll
//      have to process it further inside the helper (to
//      obtain the before/after load chains with all the
//      intermediate states we seal to to handle crashes
//      happening during the update)

type InitialProvisionParams struct {
	Key         string        `json:"key"`
	ModelParams []ModelParams `json:"model-params"`
}

// InitialProvision is called on system install to initialize
// the external fde and seal the key.
func InitialProvision(params *InitialProvisionParams, gadgetDir string) error {
	j, err := json.Marshal(params)
	if err != nil {
		return err
	}

	cmd := exec.Command(filepath.Join(gadgetDir, "fde-helper"), "--initial-provision")
	cmd.Stdin = bytes.NewReader(j)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return osutil.OutputErr(out, err)
	}

	return nil
}

type UpdateParams struct {
	ModelParams []ModelParams `json:"model-params"`
}

// Update called when there's a change in the parameters the
// key is sealed to.
func Update(params *UpdateParams) error {
	j, err := json.Marshal(params)
	if err != nil {
		return err
	}

	cmd := exec.Command("/run/mnt/ubuntu-boot/fde-helper", "--update")
	cmd.Stdin = bytes.NewReader(j)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return osutil.OutputErr(out, err)
	}

	return nil
}
