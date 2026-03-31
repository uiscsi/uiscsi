package pdu

// EncodePDU marshals a PDU into wire-ready bytes: BHS + data segment + padding.
// Digests are NOT included -- they are a transport layer concern.
func EncodePDU(p PDU) ([]byte, error) {
	bhs, err := p.MarshalBHS()
	if err != nil {
		return nil, err
	}

	data := p.DataSegment()
	dataLen := len(data)
	padLen := int(PadLen(uint32(dataLen)))

	out := make([]byte, BHSLength+dataLen+padLen)
	copy(out[:BHSLength], bhs[:])
	if dataLen > 0 {
		copy(out[BHSLength:BHSLength+dataLen], data)
	}
	// Padding bytes are already zero from make()
	return out, nil
}
