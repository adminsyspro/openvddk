# NET-0006 — A '+' in a datastore file's path

- **Why:** a report showed Open failing over NBDSSL with error 1, "disk
  chain incomplete: a file is missing", for the disk of a VM whose name
  holds a '+' (the VM's folder and files carry the name). VMware's library
  read the same disk. This record establishes how the datastore file
  service answers such a path, and what the candidate sent.
- **Reference:** VDDK 9.1.0, `libvixDiskLib.so.9.1.0.0` SHA-256
  `c955e08f…2bd9f3`, as a control only: the facts below are the file
  service's, not the library's.
- **Environment:** the vSphere 8.0 lab: vCenter 8.0.3 build 24022515,
  ESXi 8.0.3 build 24677879, a local VMFS 6 datastore. The reader was a
  Proxmox VE 9 node with nbdkit 1.42.3 and its VDDK plugin.
- **Source:** a VM created for the purpose, named `web+db`, one 1 GiB
  thin disk holding 128 MiB of random data, powered off, no snapshot: its
  disk is `[Datastore (1)] web+db/web+db.vmdk`.
- **Date:** 2026-11-11.
- **Evidence:** kept privately (the commands and their answers).

## The file service

A small file `a+b.txt` was uploaded with the path written `a%2Bb.txt`,
then read back three ways:

| Request path | vCenter 8.0.3 | ESXi 8.0.3 (direct) |
|---|---|---|
| `/folder/a%2Bb.txt` | 200 | 200 |
| `/folder/a+b.txt` | **404** | **404** |
| `/folder/a%20b.txt` | 404 | not asked |

The service decodes a '+' of the path as a space. The directory listing
writes the name back as `a%2bb.txt`.

The same test, one character at a time, for each character Go's
`net/url` leaves unescaped in a path (`! $ & ' ( ) * , ; = : @ ~`): every
one was answered 200 written as it is and 200 escaped. Only '+' differs.

## The candidate

govmomi's `Datastore.NewURL` puts the path in `url.URL.Path`, and Go
writes a '+' of a path as it is (it is valid there). The candidate read
the disk's descriptor through that URL, so the file service answered 404
and Open reported the chain broken (A10). `fileExists`, used before a
redo log is removed, answered false for such a file and the removal was
skipped.

| Library | NBDSSL | NBD |
|---|---|---|
| reference | read, SHA-256 `76a4e78a…` | read, the same |
| candidate 0.7.4 | error 1, chain incomplete | error 1, chain incomplete |
| candidate with the fix | read, the same SHA-256 | read, the same SHA-256 |

The fix sends each '+' of the path as `%2B` (`download` in
`internal/vsphere/access.go`); every other character goes out as before.
`TestFileServicePathWithPlus` serves files through a stand-in that decodes
'+' as the file service does; it fails without the fix.
