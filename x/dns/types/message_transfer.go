package types

func NewMsgTransfer(creator string, domain string, ext string, newOwner string) *MsgTransfer {
	return &MsgTransfer{
		Creator:  creator,
		Domain:   domain,
		Ext:      ext,
		NewOwner: newOwner,
	}
}
