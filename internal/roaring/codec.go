package roaring

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const magic = uint32(0xD00DF00D)

func (bm *Bitmap) WriteTo(w io.Writer) (int64, error) {
	buf := &bytes.Buffer{}

	binary.Write(buf, binary.LittleEndian, magic)
	binary.Write(buf, binary.LittleEndian, uint32(len(bm.keys)))

	for i, key := range bm.keys {
		c := bm.cons[i]
		binary.Write(buf, binary.LittleEndian, key)
		buf.WriteByte(c.kind)
		binary.Write(buf, binary.LittleEndian, uint32(c.count))

		if c.kind == kindArray {
			binary.Write(buf, binary.LittleEndian, c.arr)
		} else {
			binary.Write(buf, binary.LittleEndian, c.bits)
		}
	}

	n, err := w.Write(buf.Bytes())
	return int64(n), err
}

func (bm *Bitmap) ReadFrom(r io.Reader) (int64, error) {
	var mg uint32
	if err := binary.Read(r, binary.LittleEndian, &mg); err != nil {
		return 0, err
	}
	if mg != magic {
		return 0, fmt.Errorf("roaring: bad magic 0x%X", mg)
	}

	var numC uint32
	if err := binary.Read(r, binary.LittleEndian, &numC); err != nil {
		return 0, err
	}

	bm.keys = make([]uint16, numC)
	bm.cons = make([]*container, numC)

	for i := uint32(0); i < numC; i++ {
		var key uint16
		if err := binary.Read(r, binary.LittleEndian, &key); err != nil {
			return 0, fmt.Errorf("roaring: read key: %w", err)
		}
		var kind uint8
		if err := binary.Read(r, binary.LittleEndian, &kind); err != nil {
			return 0, fmt.Errorf("roaring: read kind: %w", err)
		}
		var count uint32
		if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
			return 0, fmt.Errorf("roaring: read count: %w", err)
		}

		c := &container{kind: kind, count: int(count)}
		if kind == kindArray {
			c.arr = make([]uint16, count)
			if err := binary.Read(r, binary.LittleEndian, c.arr); err != nil {
				return 0, fmt.Errorf("roaring: read array: %w", err)
			}
		} else {
			c.bits = make([]uint64, bitmapWords)
			if err := binary.Read(r, binary.LittleEndian, c.bits); err != nil {
				return 0, fmt.Errorf("roaring: read bitmap: %w", err)
			}
		}

		bm.keys[i] = key
		bm.cons[i] = c
	}

	return 0, nil
}

func (bm *Bitmap) Encode() []byte {
	var buf bytes.Buffer
	bm.WriteTo(&buf)
	return buf.Bytes()
}

func Decode(data []byte) (*Bitmap, error) {
	bm := New()
	_, err := bm.ReadFrom(bytes.NewReader(data))
	return bm, err
}
