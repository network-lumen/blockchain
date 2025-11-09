package types

func NewMsgUpdate(
	creator string,
	domain string,
	ext string,
	records []*Record,
	powNonce uint64,
) *MsgUpdate {
	return &MsgUpdate{
		Creator:  creator,
		Domain:   domain,
		Ext:      ext,
		Records:  records,
		PowNonce: powNonce,
	}
}
