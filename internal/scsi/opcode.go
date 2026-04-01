// Package scsi constructs SCSI Command Descriptor Blocks (CDBs) and parses
// responses for standard SCSI commands per SPC-4 and SBC-3. This is a pure
// data-transformation layer: functions take parameters and produce
// session.Command structs with correctly packed CDB bytes, or parse
// session.Result into typed Go structs.
package scsi
