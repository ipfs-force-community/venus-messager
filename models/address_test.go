package models

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
	"gorm.io/gorm"

	"github.com/filecoin-project/venus-messager/models/repo"
	"github.com/filecoin-project/venus-messager/types"
)

func TestAddress(t *testing.T) {
	sqliteRepo, mysqlRepo := setupRepo(t)

	addressRepoTest := func(t *testing.T, addressRepo repo.AddressRepo) {
		rand.Seed(time.Now().Unix())
		addr, err := address.NewIDAddress(rand.Uint64() / 2)
		assert.NoError(t, err)
		addr2, err := address.NewIDAddress(rand.Uint64() / 2)
		assert.NoError(t, err)

		addrInfo := &types.Address{
			ID:                types.NewUUID(),
			Addr:              addr,
			Nonce:             3,
			Weight:            100,
			SelMsgNum:         1,
			State:             types.Alive,
			GasOverEstimation: 1.25,
			MaxFee:            big.NewInt(10),
			MaxFeeCap:         big.NewInt(1),
			IsDeleted:         -1,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		addrInfo2 := &types.Address{
			ID:        types.NewUUID(),
			Addr:      addr2,
			SelMsgNum: 10,
			State:     types.Alive,
			MaxFee:    big.NewInt(110),
			MaxFeeCap: big.NewInt(11),
			Nonce:     2,
			IsDeleted: -1,
			CreatedAt: time.Time{},
			UpdatedAt: time.Time{},
		}

		addrInfo3 := &types.Address{
			ID:        types.NewUUID(),
			Addr:      addr,
			Nonce:     3,
			Weight:    1000,
			SelMsgNum: 10,
			State:     types.Alive,
			IsDeleted: -1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		ctx := context.Background()

		t.Run("SaveAddress", func(t *testing.T) {
			assert.NoError(t, addressRepo.SaveAddress(ctx, addrInfo))
			assert.NoError(t, addressRepo.SaveAddress(ctx, addrInfo2))
			assert.Error(t, addressRepo.SaveAddress(ctx, addrInfo3))
		})

		checkField := func(t *testing.T, expect, actual *types.Address) {
			assert.Equal(t, expect.Nonce, actual.Nonce)
			assert.Equal(t, expect.Weight, actual.Weight)
			assert.Equal(t, expect.SelMsgNum, actual.SelMsgNum)
			assert.Equal(t, expect.State, actual.State)
			assert.Equal(t, expect.GasOverEstimation, actual.GasOverEstimation)
			assert.Equal(t, expect.MaxFee, actual.MaxFee)
			assert.Equal(t, expect.MaxFeeCap, actual.MaxFeeCap)
		}

		t.Run("GetAddress", func(t *testing.T) {
			r, err := addressRepo.GetAddress(ctx, addrInfo.Addr)
			assert.NoError(t, err)
			checkField(t, addrInfo, r)

			r2, err2 := addressRepo.GetAddress(ctx, address.Undef)
			assert.True(t, xerrors.Is(err2, gorm.ErrRecordNotFound))
			assert.Contains(t, err2.Error(), gorm.ErrRecordNotFound.Error())
			assert.Nil(t, r2)
		})

		t.Run("GetAddressByID", func(t *testing.T) {
			r, err := addressRepo.GetAddressByID(ctx, addrInfo2.ID)
			assert.NoError(t, err)
			checkField(t, addrInfo2, r)
		})

		t.Run("UpdateNonce", func(t *testing.T) {
			nonce := uint64(5)
			assert.NoError(t, addressRepo.UpdateNonce(ctx, addrInfo.Addr, nonce))
			r, err := addressRepo.GetAddress(ctx, addrInfo.Addr)
			assert.NoError(t, err)
			assert.Equal(t, nonce, r.Nonce)

			r2, err2 := addressRepo.GetAddress(ctx, addrInfo3.Addr)
			assert.NoError(t, err2)
			assert.Equal(t, nonce, r2.Nonce)
		})

		t.Run("UpdateState", func(t *testing.T) {
			state := types.Forbiden
			assert.NoError(t, addressRepo.UpdateState(ctx, addrInfo.Addr, state))
			r, err := addressRepo.GetAddress(ctx, addrInfo.Addr)
			assert.NoError(t, err)
			assert.Equal(t, state, r.State)
		})

		t.Run("UpdateSelectMsgNum", func(t *testing.T) {
			num := uint64(100)
			assert.NoError(t, addressRepo.UpdateSelectMsgNum(ctx, addrInfo.Addr, num))
			r, err := addressRepo.GetAddress(ctx, addrInfo.Addr)
			assert.NoError(t, err)
			assert.Equal(t, num, r.SelMsgNum)
		})

		t.Run("UpdateFeeParams", func(t *testing.T) {
			gasOverEstimation := 1.5
			maxFeeCap := big.NewInt(1000)
			maxFee := big.NewInt(1000)
			assert.NoError(t, addressRepo.UpdateFeeParams(ctx, addr, gasOverEstimation, maxFee, maxFeeCap))

			r, err := addressRepo.GetAddress(ctx, addr)
			assert.NoError(t, err)
			assert.Equal(t, gasOverEstimation, r.GasOverEstimation)
			assert.Equal(t, maxFee, r.MaxFee)
			assert.Equal(t, maxFeeCap, r.MaxFeeCap)
		})

		t.Run("DelAddress", func(t *testing.T) {
			assert.NoError(t, addressRepo.DelAddress(ctx, addrInfo2.Addr))

			r, err := addressRepo.GetAddress(ctx, addrInfo2.Addr)
			assert.Error(t, err)
			assert.Nil(t, r)

			r, err = addressRepo.GetOneRecord(ctx, addrInfo2.Addr)
			assert.NoError(t, err)
			newAddrInfo := &types.Address{}
			*newAddrInfo = *addrInfo2
			newAddrInfo.State = types.Removed
			checkField(t, newAddrInfo, r)
		})

		t.Run("HasAddress", func(t *testing.T) {
			has, err := addressRepo.HasAddress(ctx, addrInfo.Addr)
			assert.NoError(t, err)
			assert.True(t, has)

			has, err = addressRepo.HasAddress(ctx, addrInfo2.Addr)
			assert.NoError(t, err)
			assert.False(t, has)
		})

		t.Run("ListAddress", func(t *testing.T) {
			rs, err := addressRepo.ListAddress(ctx)
			assert.NoError(t, err)
			assert.LessOrEqual(t, 1, len(rs))
		})
	}

	t.Run("TestAddress", func(t *testing.T) {
		t.Run("sqlite", func(t *testing.T) {
			addressRepoTest(t, sqliteRepo.AddressRepo())
		})
		t.Run("mysql", func(t *testing.T) {
			t.SkipNow()
			addressRepoTest(t, mysqlRepo.AddressRepo())
		})
	})
}
