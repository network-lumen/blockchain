package types

func NewMsgCreateAuction(
	creator string,
	index string,
	name string,
	start uint64,
	end uint64,
	highestBid string,
	bidder string,

) *MsgCreateAuction {
	return &MsgCreateAuction{
		Creator:    creator,
		Index:      index,
		Name:       name,
		Start:      start,
		End:        end,
		HighestBid: highestBid,
		Bidder:     bidder,
	}
}

func NewMsgUpdateAuction(
	creator string,
	index string,
	name string,
	start uint64,
	end uint64,
	highestBid string,
	bidder string,

) *MsgUpdateAuction {
	return &MsgUpdateAuction{
		Creator:    creator,
		Index:      index,
		Name:       name,
		Start:      start,
		End:        end,
		HighestBid: highestBid,
		Bidder:     bidder,
	}
}

func NewMsgDeleteAuction(
	creator string,
	index string,

) *MsgDeleteAuction {
	return &MsgDeleteAuction{
		Creator: creator,
		Index:   index,
	}
}
