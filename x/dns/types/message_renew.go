package types

func NewMsgRenew(creator string, domain string, ext string, durationDays uint64) *MsgRenew {
	return &MsgRenew{
		Creator:      creator,
		Domain:       domain,
		Ext:          ext,
		DurationDays: durationDays,
	}
}
