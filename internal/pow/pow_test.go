package pow

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tilinna/clock"
)

func TestChallengeParams(t *testing.T) {
	t.Parallel()

	tests := []challengeParams{
		{},
		{
			target:    1,
			expiresAt: 3,
		},
		{
			target:    2,
			expiresAt: -10,
			random:    []byte{0, 1, 2},
		},
		{
			random: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		},
	}

	t.Run("marshal_unmarshal", func(t *testing.T) {
		for i, test := range tests {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()
				b, err := test.MarshalBinary()
				assert.NoError(t, err)

				var c2 challengeParams
				assert.NoError(t, c2.UnmarshalBinary(b))
				assert.Equal(t, test, c2)

				b2, err := c2.MarshalBinary()
				assert.NoError(t, err)
				assert.Equal(t, b, b2)
			})
		}
	})

	secret := []byte("shhh")

	t.Run("to_from_seed", func(t *testing.T) {
		t.Parallel()

		for i, test := range tests {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()
				seed, err := newSeed(test, secret)
				assert.NoError(t, err)

				// generating seed should be deterministic
				seed2, err := newSeed(test, secret)
				assert.NoError(t, err)
				assert.Equal(t, seed, seed2)

				c, err := challengeParamsFromSeed(seed, secret)
				assert.NoError(t, err)
				assert.Equal(t, test, c)
			})
		}
	})

	t.Run("malformed_seed", func(t *testing.T) {
		t.Parallel()
		tests := []string{
			"",
			"01",
			"0000",
			"00374a1ad84d6b7a93e68042c1f850cbb100000000000000000000000000000102030405060708A0", // changed one byte from a good seed
		}

		for i, test := range tests {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()
				seed, err := hex.DecodeString(test)
				if err != nil {
					panic(err)
				}

				_, err = challengeParamsFromSeed(seed, secret)
				assert.ErrorIs(t, errMalformedSeed, err)
			})
		}
	})
}

func TestManager(t *testing.T) {
	t.Parallel()

	type testHarness struct {
		clock *clock.Mock
		mgr   Manager
	}

	newTestHarness := func(t *testing.T) *testHarness {
		t.Parallel()
		var (
			clock = clock.NewMock(time.Now().Truncate(time.Hour))
			store = NewMemoryStore(&MemoryStoreOpts{Clock: clock})
			mgr   = NewManager(store, []byte("shhhhh"), &ManagerOpts{
				Target:           0x0FFFFFFF,
				ChallengeTimeout: 1 * time.Second,
				Clock:            clock,
			})
		)

		t.Cleanup(func() { store.Close() })

		return &testHarness{clock, mgr}
	}

	t.Run("success", func(t *testing.T) {
		var (
			h        = newTestHarness(t)
			c        = h.mgr.NewChallenge()
			solution = Solve(c)
		)

		t.Log("Checking that solution starts off valid")
		assert.NoError(t, h.mgr.CheckSolution(c.Seed, solution))

		t.Log("Checking that solution continues to be valid in subsequent checks")
		assert.NoError(t, h.mgr.CheckSolution(c.Seed, solution))
	})

	t.Run("error/ErrInvalidSolution/solution too long", func(t *testing.T) {
		var (
			h        = newTestHarness(t)
			c        = h.mgr.NewChallenge()
			solution = make([]byte, len(c.Seed)+1)
		)

		assert.ErrorIs(t, h.mgr.CheckSolution(c.Seed, solution), ErrInvalidSolution)
	})

	t.Run("error/ErrInvalidSolution/solution is wrong", func(t *testing.T) {
		var (
			h        = newTestHarness(t)
			c        = h.mgr.NewChallenge()
			solution = make([]byte, len(c.Seed))
		)

		_, err := rand.Read(solution)
		require.NoError(t, err)

		assert.ErrorIs(t, h.mgr.CheckSolution(c.Seed, solution), ErrInvalidSolution)
	})

	t.Run("error/ErrExpiredSeed", func(t *testing.T) {
		var (
			h        = newTestHarness(t)
			c        = h.mgr.NewChallenge()
			solution = Solve(c)
		)

		t.Log("Checking that solution starts off valid")
		assert.NoError(t, h.mgr.CheckSolution(c.Seed, solution))

		h.clock.Add(2 * time.Second)
		t.Log("Checking that solution is no longer valid after expiry time has elapsed")
		assert.ErrorIs(t, h.mgr.CheckSolution(c.Seed, solution), ErrExpiredSeed)
	})
}
