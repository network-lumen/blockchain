package types

func NewMsgCreateDomain(
	creator string,
	index string,
	name string,
	owner string,
	records []*Record,
	expireAt uint64,

) *MsgCreateDomain {
	return &MsgCreateDomain{
		Creator:  creator,
		Index:    index,
		Name:     name,
		Owner:    owner,
		Records:  records,
		ExpireAt: expireAt,
	}
}

func NewMsgUpdateDomain(
	creator string,
	index string,
	name string,
	owner string,
	records []*Record,
	expireAt uint64,
	powNonce uint64,

) *MsgUpdateDomain {
	return &MsgUpdateDomain{
		Creator:  creator,
		Index:    index,
		Name:     name,
		Owner:    owner,
		Records:  records,
		ExpireAt: expireAt,
		PowNonce: powNonce,
	}
}

func NewMsgDeleteDomain(
	creator string,
	index string,

) *MsgDeleteDomain {
	return &MsgDeleteDomain{
		Creator: creator,
		Index:   index,
	}
}
