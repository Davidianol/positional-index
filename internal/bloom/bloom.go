package bloom

import (
	"bytes"
	"encoding/binary"
	"hash/fnv"
	"io"
	"math"
)

type BloomFilter struct {
	bitSet    []byte
	size      uint64
	hashCount uint64
}

func NewBloomFilter(m, k uint64) *BloomFilter {
	if m == 0 {
		m = 8
	}
	if k == 0 {
		k = 1
	}
	byteSize := (m + 7) / 8
	return &BloomFilter{make([]byte, byteSize), m, k}
}

func (bf *BloomFilter) Add(item []byte) {
	h1, h2 := getHashes(item)

	for i := uint64(0); i < bf.hashCount; i++ {
		pos := (h1 + i*h2) % bf.size
		byteIdx := pos / 8
		bitIdx := pos % 8

		bf.bitSet[byteIdx] |= 1 << bitIdx
	}
}

func (bf *BloomFilter) Check(item []byte) bool {
	h1, h2 := getHashes(item)

	for i := uint64(0); i < bf.hashCount; i++ {
		pos := (h1 + i*h2) % bf.size
		byteIdx := pos / 8
		bitIdx := pos % 8

		if (bf.bitSet[byteIdx] & (1 << bitIdx)) == 0 {
			return false
		}
	}
	return true
}

func OptimalParams(numElements uint64, falsePositiveRate float64) *BloomFilter {
	/*
		m - размер битового массива
		k - кол-во хэшфункций
	*/
	if numElements == 0 {
		return NewBloomFilter(8, 1)
	}
	if falsePositiveRate <= 0 {
		falsePositiveRate = 1e-9
	}
	if falsePositiveRate >= 1 {
		falsePositiveRate = 0.999999999
	}
	m := uint64(math.Ceil(float64(-numElements) * math.Log(falsePositiveRate) / (math.Pow(math.Log(2), 2))))
	k := uint64(math.Ceil(math.Log(2) * float64(m) / float64(numElements)))
	return NewBloomFilter(m, k)
}

func getHashes(data []byte) (uint64, uint64) {
	h := fnv.New64a()
	h.Write(data)
	hSum1 := h.Sum64()
	hSum2 := hSum1>>32 | hSum1<<32
	return hSum1, hSum2
}

// Encode сериализует фильтр в байты
func (bf *BloomFilter) Encode() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, bf.size)
	binary.Write(buf, binary.LittleEndian, bf.hashCount)
	buf.Write(bf.bitSet)
	return buf.Bytes()
}

// Decode восстанавливает фильтр
func Decode(data []byte) *BloomFilter {
	bf := &BloomFilter{}
	r := bytes.NewReader(data)
	_ = binary.Read(r, binary.LittleEndian, &bf.size)
	_ = binary.Read(r, binary.LittleEndian, &bf.hashCount)

	byteSize := (bf.size + 7) / 8
	bf.bitSet = make([]byte, byteSize)
	_, _ = io.ReadFull(r, bf.bitSet)
	return bf
}
