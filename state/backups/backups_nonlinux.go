// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Restore satisfies the Backups interface on non-Linux OSes (e.g.
// windows, darwin).
func (*backups) Restore(_ string, _ params.RestoreArgs) error {
	return errors.Errorf("backups supported only on Linux")
}
