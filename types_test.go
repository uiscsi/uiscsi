package uiscsi

import (
	"testing"

	"github.com/uiscsi/uiscsi/internal/scsi"
)

func TestConvertSensePropagatesFilemarkEOMILI(t *testing.T) {
	sd := &scsi.SenseData{
		ResponseCode: 0x70,
		Key:          scsi.SenseMediumError,
		ASC:          0x11,
		ASCQ:         0x00,
		Filemark:     true,
		EOM:          true,
		ILI:          true,
	}
	si := convertSense(sd)
	if !si.Filemark {
		t.Error("convertSense did not propagate Filemark=true")
	}
	if !si.EOM {
		t.Error("convertSense did not propagate EOM=true")
	}
	if !si.ILI {
		t.Error("convertSense did not propagate ILI=true")
	}

	// Test false propagation
	sd2 := &scsi.SenseData{
		ResponseCode: 0x70,
		Key:          scsi.SenseNoSense,
	}
	si2 := convertSense(sd2)
	if si2.Filemark {
		t.Error("convertSense should have Filemark=false")
	}
	if si2.EOM {
		t.Error("convertSense should have EOM=false")
	}
	if si2.ILI {
		t.Error("convertSense should have ILI=false")
	}
}
