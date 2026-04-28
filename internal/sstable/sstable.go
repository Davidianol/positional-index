package sstable

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"inverse_index/internal/bloom"
	"io"
	"os"
	"sort"
)

type IndexEntry struct {
	Key    string
	Offset int64
}

type SSTable struct {
	File      *os.File
	Bloom     *bloom.BloomFilter
	Index     []IndexEntry
	MinKey    string
	MaxKey    string
	MetaStart int64
}
type Writer struct {
	file        *os.File
	bloom       *bloom.BloomFilter
	sparseIndex []IndexEntry
	minKey      string
	maxKey      string
	itemCount   int
	curOffset   int64
}

const (
	MagicNum          = uint32(0xB000B000)
	FalsePositiveRate = float64(0.01)
	MaxKeySize        = 10000   // 10КБ
	MaxValueSize      = 1 << 20 // 1МБ
	Capacity          = 1000000
)

// Создать SSTable
func Write(filename string, data map[string]string) (*SSTable, error) {
	/*
		keys:
		len(key)-key-len(value)-value
		...
		meta:
		len(bloomFilter)-bloomFilter <- metaStartOffset
		len(sparseIndex):
		len(key)-key-offset
		...
		metaStartOffset
		MagicNum
	*/
	// Получение ключей
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Создание файла
	fi, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	// Создание блюм-фильтра и индексов
	bf := bloom.OptimalParams(uint64(len(keys)), FalsePositiveRate)
	var sparseIndex []IndexEntry

	curOffset, err := fi.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	// Запись данных
	for i, k := range keys {
		v := data[k]
		bf.Add([]byte(k))
		if i == 0 || i%100 == 0 {
			sparseIndex = append(sparseIndex, IndexEntry{
				Key:    k,
				Offset: curOffset,
			})
		}

		kBytes := []byte(k)
		vBytes := []byte(v)

		binary.Write(fi, binary.LittleEndian, uint32(len(kBytes)))
		fi.Write(kBytes)
		binary.Write(fi, binary.LittleEndian, uint32(len(vBytes)))
		fi.Write(vBytes)

		curOffset += int64(8 + len(kBytes) + len(vBytes))
	}
	// Запись метаданных
	metaStart := curOffset
	bloomBytes := bf.Encode()
	binary.Write(fi, binary.LittleEndian, uint32(len(bloomBytes)))
	fi.Write(bloomBytes)

	binary.Write(fi, binary.LittleEndian, uint32(len(sparseIndex)))

	for _, idx := range sparseIndex {
		kBytes := []byte(idx.Key)
		binary.Write(fi, binary.LittleEndian, uint32(len(kBytes)))
		fi.Write(kBytes)
		binary.Write(fi, binary.LittleEndian, idx.Offset)
	}
	binary.Write(fi, binary.LittleEndian, metaStart)
	binary.Write(fi, binary.LittleEndian, MagicNum)

	if err = fi.Sync(); err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}

	return &SSTable{
		File:      fi,
		Bloom:     bf,
		Index:     sparseIndex,
		MinKey:    keys[0],
		MaxKey:    keys[len(keys)-1],
		MetaStart: metaStart,
	}, nil
}

//	func Open(filename string) (*SSTable, error) {
//		fi, err := os.Open(filename)
//		if err != nil {
//			return nil, err
//		}
//		stat, err := fi.Stat()
//		if err != nil {
//			return nil, err
//		}
//		fileSize := stat.Size()
//		fi.Seek(fileSize-12, io.SeekStart)
//
//		var metaStart int64
//		var magic uint32
//		binary.Read(fi, binary.LittleEndian, &metaStart)
//		binary.Read(fi, binary.LittleEndian, &magic)
//
//		if magic != MagicNum {
//			return nil, fmt.Errorf("invalid magic number")
//		}
//		fi.Seek(metaStart, io.SeekStart)
//		var bloomLen uint32
//		binary.Read(fi, binary.LittleEndian, &bloomLen)
//		bloomData := make([]byte, bloomLen)
//		fi.Read(bloomData)
//		bf := bloom.Decode(bloomData)
//
//		var idxCount uint32
//		binary.Read(fi, binary.LittleEndian, &idxCount)
//		index := make([]IndexEntry, idxCount)
//		for i := 0; i < int(idxCount); i++ {
//			var Klen uint32
//			binary.Read(fi, binary.LittleEndian, &Klen)
//			kBytes := make([]byte, Klen)
//			fi.Read(kBytes)
//			var offset int64
//			binary.Read(fi, binary.LittleEndian, &offset)
//			index[i] = IndexEntry{
//				Key:    string(kBytes),
//				Offset: offset,
//			}
//		}
//		return &SSTable{
//			File:      fi,
//			Bloom:     bf,
//			Index:     index,
//			MinKey:    index[0].Key,
//			MaxKey:    index[len(index)-1].Key,
//			MetaStart: metaStart,
//		}, nil
//	}
func (sst *SSTable) Get(key string) (string, bool) {
	keyBytes := []byte(key)

	if !sst.Bloom.Check(keyBytes) {
		return "", false
	}

	idxPos := sort.Search(len(sst.Index), func(i int) bool {
		return sst.Index[i].Key > key
	})
	startIdx := idxPos - 1
	if startIdx < 0 {
		return "", false
	}
	offset := sst.Index[startIdx].Offset

	it, err := sst.NewIteratorAt(offset)
	if err != nil {
		return "", false
	}
	defer it.Close()

	for it.NextKey() {
		cmp := bytes.Compare(it.Key, keyBytes)

		if cmp == 0 {
			vb, ok := it.ReadValue()
			if !ok {
				return "", false
			}
			return string(vb), true
		}

		if cmp > 0 {
			return "", false
		}

		if !it.SkipValue() {
			return "", false
		}
	}
	return "", false
}
func (sst *SSTable) Close() error {
	if sst.File != nil {
		return sst.File.Close()
	}
	return nil
}

func NewWriter(filename string) (*Writer, error) {
	f, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	bf := bloom.OptimalParams(Capacity, FalsePositiveRate)

	return &Writer{
		file:  f,
		bloom: bf,
	}, nil
}
func (w *Writer) AppendBytes(kBytes, vBytes []byte) error {
	key := string(kBytes)
	if w.minKey == "" {
		w.minKey = key
	}
	w.maxKey = key

	w.bloom.Add(kBytes)

	if w.itemCount%100 == 0 {
		w.sparseIndex = append(w.sparseIndex, IndexEntry{Key: key, Offset: w.curOffset})
	}
	w.itemCount++

	err := binary.Write(w.file, binary.LittleEndian, uint32(len(kBytes)))
	if err != nil {
		return err
	}
	_, err = w.file.Write(kBytes)
	if err != nil {
		return err
	}
	err = binary.Write(w.file, binary.LittleEndian, uint32(len(vBytes)))
	if err != nil {
		return err
	}
	_, err = w.file.Write(vBytes)
	if err != nil {
		return err
	}

	w.curOffset += int64(8 + len(kBytes) + len(vBytes))
	return nil
}

func (w *Writer) Append(key, value string) error {
	return w.AppendBytes([]byte(key), []byte(value))
}

func (w *Writer) Close() (*SSTable, error) {
	metaStart := w.curOffset

	bloomBytes := w.bloom.Encode()
	binary.Write(w.file, binary.LittleEndian, uint32(len(bloomBytes)))
	w.file.Write(bloomBytes)

	binary.Write(w.file, binary.LittleEndian, uint32(len(w.sparseIndex)))
	for _, idx := range w.sparseIndex {
		kBytes := []byte(idx.Key)
		binary.Write(w.file, binary.LittleEndian, uint32(len(kBytes)))
		w.file.Write(kBytes)
		binary.Write(w.file, binary.LittleEndian, idx.Offset)
	}

	binary.Write(w.file, binary.LittleEndian, metaStart)
	binary.Write(w.file, binary.LittleEndian, MagicNum)

	if err := w.file.Sync(); err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}

	return &SSTable{
		File:      w.file,
		Bloom:     w.bloom,
		Index:     w.sparseIndex,
		MinKey:    w.minKey,
		MaxKey:    w.maxKey,
		MetaStart: metaStart,
	}, nil
}
