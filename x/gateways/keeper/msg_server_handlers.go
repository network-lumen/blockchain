package keeper

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"lumen/app/denom"
	"lumen/x/gateways/types"
	tokenomicstypes "lumen/x/tokenomics/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

func (m msgServer) UpdateParams(ctx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	authority, err := m.addressCodec.StringToBytes(req.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(m.GetAuthority(), authority) {
		expected, _ := m.addressCodec.BytesToString(m.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "invalid authority; expected %s, got %s", expected, req.Authority)
	}
	if err := types.ValidateParams(req.Params); err != nil {
		return nil, err
	}
	if err := m.SetParams(ctx, req.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

func (m msgServer) RegisterGateway(ctx context.Context, msg *types.MsgRegisterGateway) (*types.MsgRegisterGatewayResponse, error) {
	if _, err := m.addressCodec.StringToBytes(msg.Operator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid operator")
	}

	payout := strings.TrimSpace(msg.Payout)
	if payout == "" {
		payout = msg.Operator
	} else if _, err := m.addressCodec.StringToBytes(payout); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid payout")
	}

	metadata := strings.TrimSpace(msg.Metadata)
	if len(metadata) > 512 {
		metadata = metadata[:512]
	}

	if err := m.collectRegisterFee(ctx, msg.Operator); err != nil {
		return nil, err
	}

	id, err := m.nextGatewayID(ctx)
	if err != nil {
		return nil, err
	}

	gateway := types.Gateway{
		Id:        id,
		Operator:  msg.Operator,
		Payout:    payout,
		Active:    true,
		Metadata:  metadata,
		CreatedAt: uint64(m.nowUnix(ctx)),
	}
	if err := m.setGateway(ctx, gateway); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"gateway_register",
			sdk.NewAttribute("id", fmt.Sprintf("%d", id)),
			sdk.NewAttribute("operator", msg.Operator),
		),
	)

	return &types.MsgRegisterGatewayResponse{Id: id}, nil
}

func (m msgServer) UpdateGateway(ctx context.Context, msg *types.MsgUpdateGateway) (*types.MsgUpdateGatewayResponse, error) {
	if _, err := m.addressCodec.StringToBytes(msg.Operator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid operator")
	}
	gateway, err := m.gatewayByID(ctx, msg.GatewayId)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "gateway not found")
	}

	if err := m.collectActionFee(ctx, msg.Operator); err != nil {
		return nil, err
	}
	if gateway.Operator != msg.Operator {
		return nil, errorsmod.Wrap(types.ErrUnauthorized, "operator mismatch")
	}

	if msg.Payout != nil {
		payout := strings.TrimSpace(msg.Payout.Value)
		if payout == "" {
			return nil, errorsmod.Wrap(types.ErrInvalidRequest, "payout cannot be empty")
		}
		if _, err := m.addressCodec.StringToBytes(payout); err != nil {
			return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid payout")
		}
		gateway.Payout = payout
	}
	if msg.Metadata != nil {
		meta := strings.TrimSpace(msg.Metadata.Value)
		if len(meta) > 512 {
			meta = meta[:512]
		}
		gateway.Metadata = meta
	}
	if msg.Active != nil {
		gateway.Active = msg.Active.Value
	}

	if err := m.setGateway(ctx, gateway); err != nil {
		return nil, err
	}
	return &types.MsgUpdateGatewayResponse{}, nil
}

func (m msgServer) CreateContract(ctx context.Context, msg *types.MsgCreateContract) (*types.MsgCreateContractResponse, error) {
	if _, err := m.addressCodec.StringToBytes(msg.Client); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid client")
	}
	gateway, err := m.gatewayByID(ctx, msg.GatewayId)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "gateway not found")
	}
	if !gateway.Active {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "gateway inactive")
	}

	params := m.GetParams(ctx)
	if params.MaxActiveContractsPerGateway > 0 && gateway.ActiveClients >= params.MaxActiveContractsPerGateway {
		return nil, errorsmod.Wrapf(types.ErrOutOfBounds, "gateway reached max active contracts (%d)", params.MaxActiveContractsPerGateway)
	}

	price := sdkmath.NewIntFromUint64(msg.PriceUlmn)
	if !price.IsPositive() {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "price must be positive")
	}
	minPrice := sdkmath.NewIntFromUint64(params.MinPriceUlmnPerMonth)
	if price.LT(minPrice) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInsufficientFee, "price below minimum %d %s", params.MinPriceUlmnPerMonth, denom.BaseDenom)
	}
	if msg.MonthsTotal == 0 {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "months_total must be > 0")
	}
	total := price.MulRaw(int64(msg.MonthsTotal))
	rateBps := tokenomicstypes.DefaultTxTaxRateBps
	if m.tokenomics != nil {
		tokenomicsParams := m.tokenomics.GetParams(ctx)
		rateBps = tokenomicstypes.GetTxTaxRateBps(tokenomicsParams)
		if rateBps == 0 {
			rateBps = tokenomicstypes.DefaultTxTaxRateBps
		}
	}
	tax := total.MulRaw(int64(rateBps)).QuoRaw(10_000)
	net := total.Sub(tax)
	netPerMonth := net.QuoRaw(int64(msg.MonthsTotal))

	clientAddr, err := m.mustAddress(msg.Client)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid client address")
	}
	if err := m.moveToModule(ctx, clientAddr, authtypes.FeeCollectorName, tax); err != nil {
		return nil, err
	}
	if err := m.moveToModule(ctx, clientAddr, types.ModuleAccountEscrow, net); err != nil {
		return nil, err
	}

	id, err := m.nextContractID(ctx)
	if err != nil {
		return nil, err
	}

	now := uint64(m.nowUnix(ctx))
	var next uint64
	if params.MonthSeconds > 0 {
		next, err = m.safeAddUint64(now, params.MonthSeconds)
		if err != nil {
			return nil, err
		}
	}
	metadata := strings.TrimSpace(msg.Metadata)
	if len(metadata) > 1024 {
		metadata = metadata[:1024]
	}

	contract := types.Contract{
		Id:                id,
		Client:            msg.Client,
		GatewayId:         msg.GatewayId,
		PriceUlmn:         netPerMonth.Uint64(),
		StorageGbPerMonth: msg.StorageGbPerMonth,
		NetworkGbPerMonth: msg.NetworkGbPerMonth,
		MonthsTotal:       msg.MonthsTotal,
		StartTime:         now,
		EscrowUlmn:        net.String(),
		ClaimedMonths:     0,
		Status:            types.ContractStatus_CONTRACT_STATUS_ACTIVE,
		Metadata:          metadata,
		NextPayoutTime:    next,
	}
	if err := m.setContract(ctx, contract); err != nil {
		return nil, err
	}
	gateway.ActiveClients++
	if err := m.setGateway(ctx, gateway); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			"contract_payment",
			sdk.NewAttribute("price_ulmn", price.String()),
			sdk.NewAttribute("months_total", fmt.Sprintf("%d", msg.MonthsTotal)),
			sdk.NewAttribute("tax_ulmn", tax.String()),
			sdk.NewAttribute("net_ulmn", net.String()),
			sdk.NewAttribute("payer", msg.Client),
		),
		sdk.NewEvent(
			"contract_create",
			sdk.NewAttribute("contract_id", fmt.Sprintf("%d", id)),
			sdk.NewAttribute("gateway_id", fmt.Sprintf("%d", msg.GatewayId)),
			sdk.NewAttribute("client", msg.Client),
			sdk.NewAttribute("price_ulmn", price.String()),
			sdk.NewAttribute("months_total", fmt.Sprintf("%d", msg.MonthsTotal)),
		),
	})

	return &types.MsgCreateContractResponse{ContractId: id}, nil
}

func (m msgServer) ClaimPayment(ctx context.Context, msg *types.MsgClaimPayment) (*types.MsgClaimPaymentResponse, error) {
	if _, err := m.addressCodec.StringToBytes(msg.Operator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid operator")
	}

	contract, err := m.contractByID(ctx, msg.ContractId)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "contract not found")
	}
	gateway, err := m.gatewayByID(ctx, contract.GatewayId)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "gateway not found")
	}
	if gateway.Operator != msg.Operator {
		return nil, errorsmod.Wrap(types.ErrUnauthorized, "operator mismatch")
	}
	if contract.Status != types.ContractStatus_CONTRACT_STATUS_ACTIVE &&
		contract.Status != types.ContractStatus_CONTRACT_STATUS_COMPLETED {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "contract not active")
	}
	params := m.GetParams(ctx)
	if params.MonthSeconds == 0 {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "invalid month_seconds param")
	}
	if contract.ClaimedMonths >= contract.MonthsTotal {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "all payments claimed")
	}

	now := uint64(m.nowUnix(ctx))
	var eligible uint64
	if now > contract.StartTime {
		eligible = (now - contract.StartTime) / params.MonthSeconds
	}
	if eligible > uint64(contract.MonthsTotal) {
		eligible = uint64(contract.MonthsTotal)
	}
	claimed := uint64(contract.ClaimedMonths)
	if eligible <= claimed {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "no payout due")
	}
	monthsDue := eligible - claimed

	price := sdkmath.NewIntFromUint64(contract.PriceUlmn)
	gross := price.MulRaw(int64(monthsDue))
	if !gross.IsPositive() {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "no payout due")
	}
	escrow := m.safeAmountFromString(contract.EscrowUlmn)
	if escrow.LT(gross) {
		return nil, errorsmod.Wrap(types.ErrInsufficientFunds, "insufficient escrow")
	}

	commission := m.applyCommission(gross, params.PlatformCommissionBps)
	if commission.GT(gross) {
		return nil, errorsmod.Wrap(types.ErrOverflow, "commission overflow")
	}
	payout := gross.Sub(commission)

	payoutAddr := gateway.Payout
	if strings.TrimSpace(payoutAddr) == "" {
		payoutAddr = gateway.Operator
	}
	addr, err := m.mustAddress(payoutAddr)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid payout address")
	}

	if err := m.payFromModule(ctx, types.ModuleAccountEscrow, addr, payout); err != nil {
		return nil, err
	}
	if commission.IsPositive() {
		if err := m.moveModuleToModule(ctx, types.ModuleAccountEscrow, types.ModuleAccountTreasury, commission); err != nil {
			return nil, err
		}
	}

	contract.ClaimedMonths = uint32(claimed + monthsDue)
	contract.EscrowUlmn = escrow.Sub(gross).String()
	if contract.ClaimedMonths >= contract.MonthsTotal {
		contract.Status = types.ContractStatus_CONTRACT_STATUS_COMPLETED
		contract.NextPayoutTime = 0
	} else {
		nextMonths := uint64(contract.ClaimedMonths) + 1
		nextTime, err := m.contractOffsetTime(contract, nextMonths, params.MonthSeconds)
		if err != nil {
			return nil, err
		}
		contract.NextPayoutTime = nextTime
	}

	if err := m.setContract(ctx, contract); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"payment_claim",
			sdk.NewAttribute("contract_id", fmt.Sprintf("%d", contract.Id)),
			sdk.NewAttribute("gateway_id", fmt.Sprintf("%d", contract.GatewayId)),
			sdk.NewAttribute("operator", msg.Operator),
			sdk.NewAttribute("months_paid", fmt.Sprintf("%d", monthsDue)),
			sdk.NewAttribute("pay_amount_ulmn", payout.String()),
			sdk.NewAttribute("fee_ulmn", commission.String()),
		),
	)

	return &types.MsgClaimPaymentResponse{PaidUlmn: payout.String()}, nil
}

func (m msgServer) CancelContract(ctx context.Context, msg *types.MsgCancelContract) (*types.MsgCancelContractResponse, error) {
	if _, err := m.addressCodec.StringToBytes(msg.Client); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid client")
	}
	contract, err := m.contractByID(ctx, msg.ContractId)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "contract not found")
	}
	if contract.Client != msg.Client {
		return nil, errorsmod.Wrap(types.ErrUnauthorized, "not contract owner")
	}
	if contract.Status != types.ContractStatus_CONTRACT_STATUS_ACTIVE {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "contract not active")
	}
	if contract.ClaimedMonths >= contract.MonthsTotal {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "contract already completed")
	}

	gateway, err := m.gatewayByID(ctx, contract.GatewayId)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "gateway not found")
	}

	escrow := m.safeAmountFromString(contract.EscrowUlmn)
	price := sdkmath.NewIntFromUint64(contract.PriceUlmn)

	totalMonths := uint64(contract.MonthsTotal)
	claimedMonths := uint64(contract.ClaimedMonths)
	if claimedMonths >= totalMonths {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "nothing left to cancel")
	}
	remainingMonths := totalMonths - claimedMonths

	var refundMonths uint64
	if remainingMonths > 1 {
		refundMonths = remainingMonths - 1
	}

	refund := price.MulRaw(int64(refundMonths))
	if escrow.LT(refund) {
		return nil, errorsmod.Wrap(types.ErrInsufficientFunds, "escrow mismatch")
	}
	penalty := escrow.Sub(refund)

	clientAddr, err := m.mustAddress(msg.Client)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid client address")
	}
	if refund.IsPositive() {
		if err := m.payFromModule(ctx, types.ModuleAccountEscrow, clientAddr, refund); err != nil {
			return nil, err
		}
	}

	params := m.GetParams(ctx)

	payoutAmt := sdkmath.ZeroInt()
	feeAmt := sdkmath.ZeroInt()
	if penalty.IsPositive() {
		feeAmt = m.applyCommission(penalty, params.PlatformCommissionBps)
		if feeAmt.GT(penalty) {
			return nil, errorsmod.Wrap(types.ErrOverflow, "penalty commission overflow")
		}
		payoutAmt = penalty.Sub(feeAmt)

		payoutAddr := strings.TrimSpace(gateway.Payout)
		if payoutAddr == "" {
			payoutAddr = gateway.Operator
		}
		gatewayAddr, err := m.mustAddress(payoutAddr)
		if err != nil {
			return nil, errorsmod.Wrap(err, "invalid gateway payout address")
		}
		if err := m.payFromModule(ctx, types.ModuleAccountEscrow, gatewayAddr, payoutAmt); err != nil {
			return nil, err
		}
		if feeAmt.IsPositive() {
			if err := m.moveModuleToModule(ctx, types.ModuleAccountEscrow, types.ModuleAccountTreasury, feeAmt); err != nil {
				return nil, err
			}
		}
	}

	contract.Status = types.ContractStatus_CONTRACT_STATUS_CANCELED
	contract.EscrowUlmn = sdkmath.ZeroInt().String()
	contract.NextPayoutTime = 0

	if err := m.setContract(ctx, contract); err != nil {
		return nil, err
	}
	if gateway.ActiveClients > 0 {
		gateway.ActiveClients--
	}
	gateway.Cancellations++
	if err := m.setGateway(ctx, gateway); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"contract_cancel",
			sdk.NewAttribute("contract_id", fmt.Sprintf("%d", contract.Id)),
			sdk.NewAttribute("gateway_id", fmt.Sprintf("%d", gateway.Id)),
			sdk.NewAttribute("client", msg.Client),
			sdk.NewAttribute("refunded_ulmn", refund.String()),
			sdk.NewAttribute("payout_ulmn", payoutAmt.String()),
			sdk.NewAttribute("penalty_ulmn", penalty.String()),
			sdk.NewAttribute("fee_ulmn", feeAmt.String()),
		),
	)

	return &types.MsgCancelContractResponse{RefundedUlmn: refund.String()}, nil
}

func (m msgServer) FinalizeContract(ctx context.Context, msg *types.MsgFinalizeContract) (*types.MsgFinalizeContractResponse, error) {
	if _, err := m.addressCodec.StringToBytes(msg.Finalizer); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid finalizer")
	}
	contract, err := m.contractByID(ctx, msg.ContractId)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "contract not found")
	}
	if contract.ClaimedMonths < contract.MonthsTotal {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "payments outstanding")
	}
	if contract.Status != types.ContractStatus_CONTRACT_STATUS_COMPLETED &&
		contract.Status != types.ContractStatus_CONTRACT_STATUS_ACTIVE {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "contract not in a finalizable state")
	}

	params := m.GetParams(ctx)
	if params.MonthSeconds == 0 {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "invalid month_seconds param")
	}

	endTime, err := m.contractOffsetTime(contract, uint64(contract.MonthsTotal), params.MonthSeconds)
	if err != nil {
		return nil, err
	}
	delaySeconds, err := m.safeMulUint64(uint64(params.FinalizeDelayMonths), params.MonthSeconds)
	if err != nil {
		return nil, err
	}
	readyTime, err := m.safeAddUint64(endTime, delaySeconds)
	if err != nil {
		return nil, err
	}

	now := uint64(m.nowUnix(ctx))
	if now < readyTime {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "finalization delay not satisfied")
	}

	gateway, err := m.gatewayByID(ctx, contract.GatewayId)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "gateway not found")
	}

	escrow := m.safeAmountFromString(contract.EscrowUlmn)

	reward := m.applyCommission(escrow, params.FinalizerRewardBps)
	if reward.GT(escrow) {
		reward = escrow
	}
	refund := escrow.Sub(reward)

	finalizerAddr, err := m.mustAddress(msg.Finalizer)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid finalizer address")
	}
	if err := m.payFromModule(ctx, types.ModuleAccountEscrow, finalizerAddr, reward); err != nil {
		return nil, err
	}

	if refund.IsPositive() {
		clientAddr, err := m.mustAddress(contract.Client)
		if err != nil {
			return nil, errorsmod.Wrap(err, "invalid client address")
		}
		if err := m.payFromModule(ctx, types.ModuleAccountEscrow, clientAddr, refund); err != nil {
			return nil, err
		}
	}

	contract.Status = types.ContractStatus_CONTRACT_STATUS_FINALIZED
	contract.EscrowUlmn = sdkmath.ZeroInt().String()
	contract.NextPayoutTime = 0

	if err := m.setContract(ctx, contract); err != nil {
		return nil, err
	}
	if gateway.ActiveClients > 0 {
		gateway.ActiveClients--
	}
	if err := m.setGateway(ctx, gateway); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"contract_finalize",
			sdk.NewAttribute("contract_id", fmt.Sprintf("%d", contract.Id)),
			sdk.NewAttribute("gateway_id", fmt.Sprintf("%d", contract.GatewayId)),
			sdk.NewAttribute("finalizer", msg.Finalizer),
			sdk.NewAttribute("reward_ulmn", reward.String()),
			sdk.NewAttribute("payer_refund_ulmn", refund.String()),
		),
	)

	return &types.MsgFinalizeContractResponse{RewardUlmn: reward.String()}, nil
}
