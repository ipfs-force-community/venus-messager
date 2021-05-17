package models

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus-messager/models/repo"
	"github.com/stretchr/testify/assert"

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
			GasOverEstimation: 1.2,
			MaxFee:            big.NewInt(10),
			MaxFeeCap:         big.NewInt(20),
			IsDeleted:         -1,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		addrInfo2 := &types.Address{
			ID:        types.NewUUID(),
			Addr:      addr2,
			Nonce:     2,
			IsDeleted: -1,
			CreatedAt: time.Time{},
			UpdatedAt: time.Time{},
		}

		ctx := context.Background()

		t.Run("SaveAddress", func(t *testing.T) {
			assert.NoError(t, addressRepo.SaveAddress(ctx, addrInfo))
			assert.NoError(t, addressRepo.SaveAddress(ctx, addrInfo2))
		})

		checkField := func(t *testing.T, expect, actual *types.Address) {
			assert.Equal(t, expect.Nonce, actual.Nonce)
			assert.Equal(t, expect.Weight, actual.Weight)
			assert.Equal(t, expect.GasOverEstimation, actual.GasOverEstimation)
			assert.Equal(t, expect.MaxFee.NilOrZero(), actual.MaxFee.NilOrZero())
			assert.Equal(t, expect.MaxFeeCap.NilOrZero(), actual.MaxFeeCap.NilOrZero())
		}

		t.Run("GetAddress", func(t *testing.T) {
			r, err := addressRepo.GetAddress(ctx, addr)
			assert.NoError(t, err)
			checkField(t, addrInfo, r)
		})

		t.Run("GetAddressByID", func(t *testing.T) {
			r, err := addressRepo.GetAddressByID(ctx, addrInfo2.ID)
			assert.NoError(t, err)
			checkField(t, addrInfo2, r)
		})

		t.Run("UpdateNonce", func(t *testing.T) {
			newNonce := uint64(5)
			assert.NoError(t, addressRepo.UpdateNonce(ctx, addr, newNonce))
			r2, err := addressRepo.GetAddress(ctx, addr)
			assert.NoError(t, err)
			assert.Equal(t, newNonce, r2.Nonce)
		})

		t.Run("DelAddress", func(t *testing.T) {
			assert.NoError(t, addressRepo.DelAddress(ctx, addr2))

			r, err := addressRepo.GetAddress(ctx, addr2)
			assert.Error(t, err)
			assert.Nil(t, r)

			r, err = addressRepo.GetOneRecord(ctx, addr2)
			assert.NoError(t, err)
			checkField(t, addrInfo2, r)
		})

		t.Run("HasAddress", func(t *testing.T) {
			has, err := addressRepo.HasAddress(ctx, addr)
			assert.NoError(t, err)
			assert.True(t, has)

			has, err = addressRepo.HasAddress(ctx, addr2)
			assert.NoError(t, err)
			assert.True(t, has)
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
