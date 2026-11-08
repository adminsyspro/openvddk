# NBD-0032 — ESXi 7.0.0 GA: NFC version 5, no features or resource pool

- **Why:** a user reported that 0.7.4 cannot open a disk on a standalone
  ESXi 7.0.0 GA host (issue #1): every `VixDiskLib_Open` fails with
  `nfc: unexpected reply from the host: bad frame magic 0x4`, over NBD and
  NBDSSL. Earlier direct ESXi checks covered 6.7 U3 and 7.0 U3c only.
- **Environment:** ESXi 7.0.0 build 15843807 (7.0 GA), standalone, on an
  evaluation licence, nested on QEMU, local VMFS 6 datastore. Test VM
  powered off, no changed block tracking, two thin disks of 2 GiB and
  4 GiB.
- **Reader:** Proxmox VE 9 node, nbdkit 1.42.3 with its VDDK plugin.
- **Reference:** VMware VDDK 8.0.3, used as a comparison only.
- **Candidates:** 0.7.4 (`94926da2f764`) and the change below.
- **Date:** 2026-11-07 and 2026-11-08.
- **Evidence:** captures of port 902 over NBD, kept privately.

## What the host does

Asked with version 11, the host answers NFC version **5**. 0.7.4 then sends
connection data, features (55) and the AIO session request, the open
session (AIO 2) and socket options (AIO 9) requests, which the host
answers, and the resource pool request (AIO 22, `uint32` 1). The host
answers that request with a classic session complete message (type 4) and
closes the connection, hence the error.

To the same host the reference also asks version 11 and gets 5. It sends
connection data but neither features nor the resource pool request: after
the open session and socket options requests it opens the file (AIO 4)
directly, and it reads both disks to the end over NBD and NBDSSL.

With the observations so far:

| Host | NFC version | Connection data | Features, resource pool |
|---|---|---|---|
| ESXi 6.7 U3 (NBD-0005) | 2 | no | no |
| ESXi 7.0.0 GA (this record) | 5 | yes | no |
| ESXi 7.0.3 (NET-0003) | 9 | yes | yes |

## Change

Connection data is still sent from version 3. Features and the resource
pool request are sent from version 9, the lowest version on which the
reference was seen sending them. Leaving them out on a host between 5 and
9 costs the resource pool tag only, which the reference does without on
7.0.0 GA; sending them to a host that does not take them ends the session.

## Results with the change, against the 7.0.0 GA host

- `nbdinfo --size` over NBD and NBDSSL: both open, 2,147,483,648 bytes.
- 96 MiB of random data written into the first disk with the reference
  over NBD, then a whole-disk read with `nbdcopy`: the same SHA-256
  (`ad5f47de…`) with the candidate over NBD and NBDSSL and with the
  reference over NBDSSL; `nbdinfo --map` gives the same map with all
  three.
- With a snapshot (`snapshot=` of the plugin), both disks at the snapshot
  with `transports=nbdssl:nbd`: the same SHA-256 with the candidate and the
  reference (`ad5f47de…`, `8479e439…`).
- Capture over NBD: connection data, then the AIO session, open session
  (2), socket options (9), open file (4), allocation query (13), close
  file (5) and close session (3); no features and no AIO 22.
- `go test ./internal/...` passes; the new `TestESXi70GASession` fails
  without the change.
