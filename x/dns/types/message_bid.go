package types

func NewMsgBid(creator string, domain string, ext string, amount string) *MsgBid {
	return &MsgBid{
		Creator: creator,
		Domain:  domain,
		Ext:     ext,
		Amount:  amount,
	}
}
