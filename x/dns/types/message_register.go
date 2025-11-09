package types

func NewMsgRegister(
	creator string,
	domain string,
	ext string,
	records []*Record,
	durationDays uint64,
) *MsgRegister {
	return &MsgRegister{
		Creator:      creator,
		Domain:       domain,
		Ext:          ext,
		Records:      records,
		DurationDays: durationDays,
	}
}
