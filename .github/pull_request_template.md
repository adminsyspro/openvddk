## What and why

<!-- What does this change do, and which issue does it address (#...)? -->

## Tested against

<!-- ESXi and vCenter versions (and the NFC version from the log, where it
matters), transports (NBD, NBDSSL, HotAdd), and how you tested. -->

## Checklist

- [ ] `tools/container/run.sh make check` passes
- [ ] Every commit has a `Signed-off-by` line (`git commit -s`)
- [ ] Clean room: no code, disassembler or decompiler output, message text
      or files from VMware's libraries or other proprietary software; every
      new fact about the reference library's behaviour names its source
      (CONTRIBUTING.md)
- [ ] A change of behaviour comes with a test
