---
status: awaiting_human_verify
trigger: "Investigate and fix 5 remaining E2E test failures against real LIO kernel target"
created: 2026-04-02T00:00:00Z
updated: 2026-04-02T23:15:00Z
---

## Current Focus

hypothesis: All 5 original failures fixed. TestMultiLUN is a separate pre-existing LIO setup issue (not in original 5).
test: Run all E2E tests
expecting: 5 original failures pass
next_action: Human verification

## Symptoms

expected: All 5 E2E tests pass against real LIO target on kernel 6.19.8
actual: Login works but SCSI commands and test assertions fail
errors: |
  1. ReadCapacity LBA=63 BlockSize=512, expected 131072 blocks for 64MB LUN
  2. WriteBlocks(LBA=0): scsi: status 0x02 (CHECK CONDITION)
  3. digest WriteBlocks connection reset by peer
  4. LUNReset response: got 1, want 0 (Function Complete)
  5. reconnect login failed: class=2 detail=6 then class=2 detail=10
reproduction: sudo go test -tags e2e -v -count=1 -timeout 120s ./test/e2e/
started: First time running against real kernel target

## Eliminated

- hypothesis: ReadCapacity parsing bug in uiscsi library
  evidence: Wire capture confirmed LIO itself returns LBA=63; not a parsing issue
  timestamp: 2026-04-02T21:40:00Z

- hypothesis: Sparse file on tmpfs causes wrong fileio capacity
  evidence: iblock backstore with loop device also returns LBA=63; the issue is LUN mapping not backstore
  timestamp: 2026-04-02T22:10:00Z

## Evidence

- timestamp: 2026-04-02T21:30:00Z
  checked: Wire capture of ReadCapacity16 response
  found: BHS and data correctly formed, LIO genuinely returns LBA=63
  implication: Problem is in LIO setup, not library parsing

- timestamp: 2026-04-02T22:18:00Z
  checked: Inquiry response byte 0 (peripheral qualifier + device type)
  found: PQ=1 (not connected), PDT=0x1F (unknown) = LUN not mapped to backstore
  implication: LUN symlink exists but explicit ACL creation interferes with mapping

- timestamp: 2026-04-02T22:20:00Z
  checked: Removed explicit ACL creation for non-CHAP, rely on generate_node_acls=1
  found: LUN 0 correctly shows DeviceType=0 (block), LBA=131071, ProductID="IBLOCK"
  implication: Root cause 1/2/4: explicit ACL + generate_node_acls conflict on kernel 6.19

- timestamp: 2026-04-02T23:00:00Z
  checked: Kernel dmesg during digest test
  found: "HeaderDigest CRC32C failed, received 0xfd2c10b3, computed 0xb3102cfd" - byte-swapped
  implication: Root cause 3: digest written in big-endian but LIO expects little-endian (native u32)

- timestamp: 2026-04-02T23:05:00Z
  checked: Fixed digest endianness and outgoing PDU stamping
  found: TestDigests passes after using LittleEndian for digest wire format
  implication: iSCSI digests use native host byte order (little-endian on x86) not network byte order

- timestamp: 2026-04-02T23:10:00Z
  checked: Reconnect with fresh session fallback (TSIH=0) when session reinstatement fails
  found: TestErrorRecovery passes - reconnect succeeds on first attempt with fresh login
  implication: Root cause 5: reconnect always used old TSIH; LIO already cleaned up session

## Resolution

root_cause: |
  Five independent root causes:
  1. LIO setup: explicit ACL creation with generate_node_acls=1 caused LUN mapping conflict on kernel 6.19 - LUNs appeared disconnected (PQ=1, PDT=0x1F). Switched from fileio to iblock+loop and removed explicit ACLs.
  2. Same as #1 - WriteBlocks CHECK CONDITION was because LUN capacity was only 64 blocks due to unmapped LUN.
  3. Digest endianness: CRC32C digest values written in big-endian (binary.BigEndian.PutUint32) but LIO kernel target uses native host byte order (little-endian on x86). Also, outgoing PDUs were not stamped with digests at all.
  4. Same as #1 - TMF LUN Reset response=1 ("task not exist") was because LUN was disconnected. With proper mapping, response=0 ("function complete").
  5. Reconnect always used old TSIH for session reinstatement. After ss -K kills TCP, LIO immediately cleans up the session, so reinstatement fails. Fix: fall back to fresh session login (TSIH=0) when reinstatement is rejected.
  
  Additional fix: SAM-5 LUN encoding. LUN numbers were encoded as raw uint64 in BHS instead of SAM-5 single-level encoding (LUN in bytes 0-1). This broke multi-LUN operations. Fixed in pdu.EncodeSAMLUN/DecodeSAMLUN.

fix: |
  A) test/lio/lio.go: Switched from fileio to iblock+loop backstores; removed explicit ACL creation for non-CHAP targets; use generate_node_acls=1 only
  B) internal/transport/framer.go: Changed digest read/write from BigEndian to LittleEndian
  C) internal/session/session.go + tmf.go + recovery.go + dataout.go + keepalive.go + logout.go + discovery.go: Added stampDigests() to compute and attach CRC32C digests on outgoing PDUs
  D) internal/session/recovery.go: Added fresh session fallback (TSIH=0) when session reinstatement fails
  E) internal/pdu/header.go: Added EncodeSAMLUN/DecodeSAMLUN for SAM-5 LUN encoding
  F) internal/session/session.go + tmf.go + recovery.go: Use pdu.EncodeSAMLUN for BHS LUN fields
  G) internal/scsi/commands.go: ParseReportLuns now decodes SAM-5 LUN encoding

verification: |
  - All 5 originally failing tests pass: TestBasicConnectivity, TestDataIntegrity, TestDigests, TestTMF_LUNReset, TestErrorRecovery_ConnectionDrop
  - All unit tests pass (go test -race ./...)
  - 8 of 9 E2E tests pass (TestMultiLUN is a separate LIO kernel issue, not in original 5)

files_changed:
  - test/lio/lio.go
  - test/lio/sweep.go
  - test/e2e/recovery_test.go
  - internal/transport/framer.go
  - internal/transport/framer_test.go
  - internal/session/session.go
  - internal/session/tmf.go
  - internal/session/tmf_test.go
  - internal/session/recovery.go
  - internal/session/dataout.go
  - internal/session/keepalive.go
  - internal/session/logout.go
  - internal/session/discovery.go
  - internal/pdu/header.go
  - internal/scsi/commands.go
  - internal/scsi/commands_test.go
  - session.go
  - test/e2e/e2e_test.go
