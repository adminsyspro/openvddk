// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// ErrAlreadyDisabled reports that access preparation is already in effect.
var ErrAlreadyDisabled = errors.New("relocation is already disabled for this virtual machine")

// relocateMethod is the only method the reference disables (HOTADD-0002).
const relocateMethod = "RelocateVM_Task"

// DisableRelocation implements PrepareForAccess: relocation (Storage
// vMotion) of the VM is disabled with identity as the disabling source.
func (s *Session) DisableRelocation(ctx context.Context, vm, identity string) error {
	if err := s.vmExists(ctx, vm); err != nil {
		return err
	}
	err := disableMethods(ctx, s.c, *s.c.ServiceContent.AuthorizationManager,
		[]types.ManagedObjectReference{vmRef(vm)}, []string{relocateMethod}, identity)
	if err != nil {
		if fault.Is(err, &types.MethodAlreadyDisabledFault{}) {
			return fmt.Errorf("%w: %v", ErrAlreadyDisabled, err)
		}
		return withPermission(err, "PrepareForAccess")
	}
	return nil
}

// EnableRelocation implements EndAccess.
func (s *Session) EnableRelocation(ctx context.Context, vm, identity string) error {
	return withPermission(enableMethods(ctx, s.c, *s.c.ServiceContent.AuthorizationManager,
		[]types.ManagedObjectReference{vmRef(vm)}, []string{relocateMethod}, identity), "EndAccess")
}

func (s *Session) vmExists(ctx context.Context, vm string) error {
	var v mo.VirtualMachine
	if err := property.DefaultCollector(s.c).RetrieveOne(ctx, vmRef(vm), []string{"name"}, &v); err != nil {
		if isNotFound(err) {
			return fmt.Errorf("%w: %s", ErrVMNotFound, vm)
		}
		return err
	}
	return nil
}

// MaxDescriptorBytes bounds a descriptor download.
const MaxDescriptorBytes = 64 << 10

// ReadDescriptor downloads the first bytes of a disk's descriptor file
// through the datastore file service.
// ErrFileNotFound: the datastore has no file at the path asked for.
var ErrFileNotFound = errors.New("file not found on the datastore")

func (s *Session) ReadDescriptor(ctx context.Context, datastore, path string) ([]byte, error) {
	_, rel := splitDatastorePath(path)
	ds := object.NewDatastore(s.c, types.ManagedObjectReference{Type: "Datastore", Value: datastore})
	if err := ds.FindInventoryPath(ctx); err != nil {
		return nil, err
	}
	rc, err := download(ctx, ds, rel, soap.DefaultDownload)
	if err != nil {
		notFound := strings.Contains(err.Error(), "404 Not Found")
		if notFound || strings.Contains(err.Error(), "401 Unauthorized") {
			// The file service answers 404 when the user may not browse
			// the datastore and 401 when it may not use low level file
			// operations on it: say which privilege is missing (V1, D-0032).
			if priv, perr := s.missingPrivilege(ctx, ds.Reference(), datastoreFilePrivileges...); perr == nil && priv != "" {
				return nil, fmt.Errorf("%w (read the disk's descriptor): needs %s on %s", ErrNoPermission, priv, datastore)
			}
		}
		if notFound {
			return nil, fmt.Errorf("%w: %v", ErrFileNotFound, err)
		}
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, MaxDescriptorBytes))
}

// download is Datastore.Download with each '+' of the file's path sent as
// %2B. Go leaves '+' as it is in a URL path, where it is valid, but the
// datastore file service of vCenter and ESXi reads it as a space: a file
// whose name holds '+' (a VM named "a+b" keeps its disk in a+b/a+b.vmdk)
// was answered 404 and taken for missing (NET-0006). Every other character
// goes out as before.
func download(ctx context.Context, ds *object.Datastore, rel string, p soap.Download) (io.ReadCloser, error) {
	u, ticket, err := ds.ServiceTicket(ctx, rel, p.Method)
	if err != nil {
		return nil, err
	}
	if ticket != nil {
		p.Ticket = ticket
		p.Close = true // as Datastore.Download does: no Keep-Alive to the host
	}
	u.RawPath = plusEscaped(u.EscapedPath())
	rc, _, err := ds.Client().Download(ctx, u, &p)
	return rc, err
}

// plusEscaped returns an escaped URL path with each '+' written %2B.
func plusEscaped(escapedPath string) string {
	return strings.ReplaceAll(escapedPath, "+", "%2B")
}

// datastoreFilePrivileges are what the datastore file service checks: with
// either missing, it refuses every read of the datastore's files (V1).
var datastoreFilePrivileges = []string{"Datastore.Browse", "Datastore.FileManagement"}

// missingPrivilege asks vCenter which of privs the signed-in user lacks on
// entity; "" when it holds them all.
func (s *Session) missingPrivilege(ctx context.Context, entity types.ManagedObjectReference, privs ...string) (string, error) {
	us, err := s.sm.UserSession(ctx)
	if err != nil || us == nil {
		return "", fmt.Errorf("user session unknown: %v", err)
	}
	res, err := methods.HasUserPrivilegeOnEntities(ctx, s.c, &types.HasUserPrivilegeOnEntities{
		This: *s.c.ServiceContent.AuthorizationManager, Entities: []types.ManagedObjectReference{entity},
		UserName: us.UserName, PrivId: privs,
	})
	if err != nil {
		return "", err
	}
	return firstMissing(privs, res.Returnval)
}

// firstMissing reads vCenter's answer to HasUserPrivilegeOnEntities: the
// first of privs not granted, "" when all are.
func firstMissing(privs []string, answer []types.EntityPrivilege) (string, error) {
	granted := map[string]bool{}
	for _, e := range answer {
		for _, p := range e.PrivAvailability {
			granted[p.PrivId] = p.IsGranted
		}
	}
	for _, p := range privs {
		g, ok := granted[p]
		if !ok {
			return "", fmt.Errorf("vCenter gave no answer for %s", p)
		}
		if !g {
			return p, nil
		}
	}
	return "", nil
}
