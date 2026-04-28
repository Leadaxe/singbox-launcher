package v5

import (
	"crypto/rand"
	"sync"
	"time"
)

// crockfordAlphabet — Crockford base32 (no I, L, O, U).
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ulidLen — длина строкового ULID: 10 символов timestamp (48 бит)
// + 16 символов рандома (80 бит) = 26 символов всего.
const ulidLen = 26

var (
	ulidMu   sync.Mutex
	lastTime int64
	lastRand [10]byte
)

// MakeULID возвращает новый ULID (26 знаков Crockford base32).
//
// 48-bit ms timestamp + 80-bit cryptographic random. Монотонность
// внутри одной миллисекунды поддерживается инкрементом random части
// (как в spec'е ULID), что гарантирует строгий лексикографический
// порядок без коллизий при batch-генерации.
func MakeULID() string {
	ulidMu.Lock()
	defer ulidMu.Unlock()

	now := time.Now().UTC().UnixMilli()

	var rnd [10]byte
	if now == lastTime {
		// Та же миллисекунда → инкремент предыдущего random для монотонности.
		rnd = lastRand
		incrementBytes(rnd[:])
	} else {
		if _, err := rand.Read(rnd[:]); err != nil {
			// crypto/rand не должен падать; на крайний случай —
			// fallback на time-derived bytes (тесты остаются детерминированы
			// своим IDGen'ом, prod path не зависит от этого).
			ts := uint64(now)
			for i := 0; i < 10; i++ {
				rnd[i] = byte(ts >> (uint(i) * 7))
			}
		}
		lastTime = now
	}
	lastRand = rnd

	out := make([]byte, ulidLen)
	encodeTimestamp(uint64(now), out[:10])
	encodeRandom(rnd[:], out[10:])
	return string(out)
}

func encodeTimestamp(ts uint64, dst []byte) {
	for i := 9; i >= 0; i-- {
		dst[i] = crockfordAlphabet[ts&0x1F]
		ts >>= 5
	}
}

func encodeRandom(src, dst []byte) {
	// 10 bytes (80 bits) → 16 base32 chars (5 bits each).
	dst[0] = crockfordAlphabet[(src[0]&0xF8)>>3]
	dst[1] = crockfordAlphabet[((src[0]&0x07)<<2)|((src[1]&0xC0)>>6)]
	dst[2] = crockfordAlphabet[(src[1]&0x3E)>>1]
	dst[3] = crockfordAlphabet[((src[1]&0x01)<<4)|((src[2]&0xF0)>>4)]
	dst[4] = crockfordAlphabet[((src[2]&0x0F)<<1)|((src[3]&0x80)>>7)]
	dst[5] = crockfordAlphabet[(src[3]&0x7C)>>2]
	dst[6] = crockfordAlphabet[((src[3]&0x03)<<3)|((src[4]&0xE0)>>5)]
	dst[7] = crockfordAlphabet[src[4]&0x1F]
	dst[8] = crockfordAlphabet[(src[5]&0xF8)>>3]
	dst[9] = crockfordAlphabet[((src[5]&0x07)<<2)|((src[6]&0xC0)>>6)]
	dst[10] = crockfordAlphabet[(src[6]&0x3E)>>1]
	dst[11] = crockfordAlphabet[((src[6]&0x01)<<4)|((src[7]&0xF0)>>4)]
	dst[12] = crockfordAlphabet[((src[7]&0x0F)<<1)|((src[8]&0x80)>>7)]
	dst[13] = crockfordAlphabet[(src[8]&0x7C)>>2]
	dst[14] = crockfordAlphabet[((src[8]&0x03)<<3)|((src[9]&0xE0)>>5)]
	dst[15] = crockfordAlphabet[src[9]&0x1F]
}

func incrementBytes(b []byte) {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return
		}
	}
}
