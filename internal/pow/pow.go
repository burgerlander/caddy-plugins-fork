// Package pow creates proof-of-work challenges and validates their solutions.
package pow

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"sync"
	"time"

	"github.com/tilinna/clock"
)

type challengeParams struct {
	target    uint32
	expiresAt int64
	random    []byte
}

func (c challengeParams) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	var err error
	write := func(v interface{}) {
		if err != nil {
			return
		}
		err = binary.Write(buf, binary.BigEndian, v)
	}

	write(c.target)
	write(c.expiresAt)

	if err != nil {
		return nil, err
	}

	if _, err := buf.Write(c.random); err != nil {
		panic(err)
	}

	return buf.Bytes(), nil
}

func (c *challengeParams) UnmarshalBinary(b []byte) error {
	buf := bytes.NewBuffer(b)

	var err error
	read := func(into interface{}) {
		if err != nil {
			return
		}
		err = binary.Read(buf, binary.BigEndian, into)
	}

	read(&c.target)
	read(&c.expiresAt)

	if buf.Len() > 0 {
		c.random = buf.Bytes() // whatever is left
	}

	return err
}

// The seed takes the form:
//
//	(version)+(signature of challengeParams)+(challengeParams)
//
// Version is currently always 0.
func newSeed(c challengeParams, secret []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.WriteByte(0) // version

	cb, err := c.MarshalBinary()
	if err != nil {
		return nil, err
	}

	h := hmac.New(md5.New, secret)
	h.Write(cb)
	buf.Write(h.Sum(nil))

	buf.Write(cb)

	return buf.Bytes(), nil
}

var errMalformedSeed = errors.New("malformed seed")

func challengeParamsFromSeed(seed, secret []byte) (challengeParams, error) {
	h := hmac.New(md5.New, secret)
	hSize := h.Size()

	if len(seed) < hSize+1 || seed[0] != 0 {
		return challengeParams{}, errMalformedSeed
	}
	seed = seed[1:]

	sig, cb := seed[:hSize], seed[hSize:]

	// check signature
	h.Write(cb)
	if !hmac.Equal(sig, h.Sum(nil)) {
		return challengeParams{}, errMalformedSeed
	}

	var c challengeParams
	if err := c.UnmarshalBinary(cb); err != nil {
		return challengeParams{}, fmt.Errorf("unmarshaling challenge parameters: %w", err)
	}

	return c, nil
}

// Challenge is a set of fields presented to a client, with which they must
// generate a solution.
//
// Generating a solution is done by:
//
//   - Collect up to len(Seed) random bytes. These will be the potential
//     solution.
//
//   - Calculate the sha512 of the concatenation of Seed and PotentialSolution.
//
//   - Parse the first 4 bytes of the sha512 result as a big-endian uint32.
//
//   - If the resulting number is _less_ than target, the solution has been
//     found. Otherwise go back to step 1 and try again.
type Challenge struct {
	Seed   []byte
	Target uint32
}

// Errors which may be produced by a Manager.
var (
	ErrInvalidSolution = errors.New("invalid solution")
	ErrExpiredSeed     = errors.New("expired seed")
)

// Manager is used to both produce proof-of-work challenges and check their
// solutions.
type Manager interface {
	NewChallenge() Challenge

	// Will produce ErrInvalidSolution if the solution is invalid, or
	// ErrExpiredSeed if the seed has expired.
	CheckSolution(seed, solution []byte) error
}

// ManagerParams are used to initialize a new Manager instance. All fields are
// required unless otherwise noted.
type ManagerParams struct {
	Store Store

	// Secret is used to sign each Challenge's Seed, it should _not_ be shared
	// with clients.
	Secret []byte
}

// ManagerOpts are optional parameters to the NewManager function. A nil value
// is equivalent to a zero value.
type ManagerOpts struct {

	// Target indicates how difficult each Challenge will be to solve. A _lower_
	// Target value is more difficult than a higher one.
	//
	// Defaults to 0x00FFFFFF
	Target uint32

	// ChallengeTimeout indicates how long before Challenges are considered
	// expired and cannot be solved.
	//
	// Defaults to 12 hours.
	ChallengeTimeout time.Duration

	// Clock is used for controlling the view of time.
	//
	// Defaults to clock.Realtime().
	Clock clock.Clock
}

func (o *ManagerOpts) withDefaults() *ManagerOpts {
	if o == nil {
		o = new(ManagerOpts)
	}

	if o.Target == 0 {
		o.Target = 0x00FFFFFF
	}

	if o.ChallengeTimeout == 0 {
		o.ChallengeTimeout = 12 * time.Hour
	}

	if o.Clock == nil {
		o.Clock = clock.Realtime()
	}

	return o
}

type manager struct {
	store               Store
	secret              []byte
	opts                *ManagerOpts
	solutionCheckerPool sync.Pool
}

// NewManager initializes and returns a Manager instance using the given
// parameters.
//
// The secret is used to sign the seed values and should never be shared with
// clients.
func NewManager(store Store, secret []byte, opts *ManagerOpts) Manager {
	return &manager{
		store,
		secret,
		opts.withDefaults(),
		sync.Pool{
			New: func() any { return SolutionChecker{} },
		},
	}
}

func (m *manager) NewChallenge() Challenge {
	c := challengeParams{
		target:    m.opts.Target,
		expiresAt: m.opts.Clock.Now().Add(m.opts.ChallengeTimeout).Unix(),
		random:    make([]byte, 8),
	}

	if _, err := rand.Read(c.random); err != nil {
		panic(err)
	}

	seed, err := newSeed(c, m.secret)
	if err != nil {
		panic(err)
	}

	return Challenge{
		Seed:   seed,
		Target: c.target,
	}
}

// SolutionChecker can be used to check possible Challenge solutions. It will
// cache certain values internally to save on allocations when used in a loop
// (e.g. when generating a solution).
//
// SolutionChecker is not thread-safe.
type SolutionChecker struct {
	h   hash.Hash // sha512
	sum []byte
}

// Check returns true if the given bytes are a solution to the given Challenge.
func (s SolutionChecker) Check(challenge Challenge, solution []byte) bool {
	if s.h == nil {
		s.h = sha512.New()
	}
	s.h.Reset()

	s.h.Write(challenge.Seed)
	s.h.Write(solution)
	s.sum = s.h.Sum(s.sum[:0])

	i := binary.BigEndian.Uint32(s.sum[:4])
	return i < challenge.Target
}

func (m *manager) CheckSolution(seed, solution []byte) error {
	if len(solution) > len(seed) {
		return ErrInvalidSolution
	}

	if m.store.IsSolution(seed, solution) {
		return nil
	}

	c, err := challengeParamsFromSeed(seed, m.secret)
	if err != nil {
		return fmt.Errorf("parsing challenge parameters from seed: %w", err)

	} else if now := m.opts.Clock.Now().Unix(); c.expiresAt <= now {
		return ErrExpiredSeed
	}

	solutionChecker := m.solutionCheckerPool.Get().(SolutionChecker)
	defer m.solutionCheckerPool.Put(solutionChecker)

	ok := solutionChecker.Check(
		Challenge{Seed: seed, Target: c.target}, solution,
	)

	if !ok {
		return ErrInvalidSolution
	}

	expiresAt := time.Unix(c.expiresAt, 0)
	if err := m.store.SetSolution(seed, solution, expiresAt); err != nil {
		return fmt.Errorf("marking solution as solved: %w", err)
	}

	return nil
}

// Solve returns a solution for the given Challenge. This may take a while.
func Solve(challenge Challenge) []byte {
	var (
		chk = SolutionChecker{}
		b   = make([]byte, len(challenge.Seed))
	)

	for {
		if _, err := rand.Read(b); err != nil {
			panic(err)
		} else if chk.Check(challenge, b) {
			return b
		}
	}
}
