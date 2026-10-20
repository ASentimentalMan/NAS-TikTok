package sampling

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	mathrand "math/rand"
)

// Reservoir keeps a uniformly selected sample while candidates arrive once.
type Reservoir struct {
	limit int
	seen  int64
	items []string
	rng   *mathrand.Rand
}

func New(limit int) (*Reservoir, error) {
	if limit <= 0 {
		return nil, errors.New("sample limit must be positive")
	}
	var seedBytes [8]byte
	if _, err := rand.Read(seedBytes[:]); err != nil {
		return nil, errors.New("random sampling is unavailable")
	}
	seed := int64(binary.LittleEndian.Uint64(seedBytes[:]) & 0x7fffffffffffffff)
	return &Reservoir{
		limit: limit,
		items: make([]string, 0, limit),
		rng:   mathrand.New(mathrand.NewSource(seed)),
	}, nil
}

func (reservoir *Reservoir) Add(candidate string) {
	reservoir.seen++
	if len(reservoir.items) < reservoir.limit {
		reservoir.items = append(reservoir.items, candidate)
		return
	}
	index := reservoir.rng.Int63n(reservoir.seen)
	if index < int64(reservoir.limit) {
		reservoir.items[index] = candidate
	}
}

func (reservoir *Reservoir) Items() []string {
	return append([]string(nil), reservoir.items...)
}
