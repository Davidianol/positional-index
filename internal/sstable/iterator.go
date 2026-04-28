package sstable

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type Iterator struct {
	f  *os.File
	br *bufio.Reader

	keyBuf []byte
	valBuf []byte

	Key []byte

	pendingValLen uint32
	pendingValue  bool

	err error
	eof bool
}

func (sst *SSTable) NewIterator() (*Iterator, error) {
	return sst.NewIteratorAt(0)
}

func (sst *SSTable) NewIteratorAt(offset int64) (*Iterator, error) {
	if sst.MetaStart <= 0 {
		return nil, fmt.Errorf("invalid MetaStart=%d", sst.MetaStart)
	}
	if offset < 0 || offset > sst.MetaStart {
		return nil, fmt.Errorf("bad offset=%d (MetaStart=%d)", offset, sst.MetaStart)
	}

	path := sst.File.Name()
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	sr := io.NewSectionReader(f, offset, sst.MetaStart-offset)
	br := bufio.NewReaderSize(sr, 64*1024)

	return &Iterator{f: f, br: br}, nil
}

func (it *Iterator) Close() error {
	if it.f != nil {
		return it.f.Close()
	}
	return nil
}

func (it *Iterator) Error() error { return it.err }

func (it *Iterator) NextKey() bool {
	if it.err != nil || it.eof {
		return false
	}

	if it.pendingValue {
		if !it.SkipValue() {
			return false
		}
	}

	var kLen uint32
	if err := binary.Read(it.br, binary.LittleEndian, &kLen); err != nil {
		if err == io.EOF {
			it.eof = true
			return false
		}
		it.err = err
		return false
	}
	if kLen == 0 || kLen > MaxKeySize {
		it.err = fmt.Errorf("bad key length: %d", kLen)
		return false
	}

	if cap(it.keyBuf) < int(kLen) {
		it.keyBuf = make([]byte, kLen)
	} else {
		it.keyBuf = it.keyBuf[:kLen]
	}
	if _, err := io.ReadFull(it.br, it.keyBuf); err != nil {
		it.err = err
		return false
	}
	it.Key = it.keyBuf

	var vLen uint32
	if err := binary.Read(it.br, binary.LittleEndian, &vLen); err != nil {
		it.err = err
		return false
	}
	if vLen > MaxValueSize {
		it.err = fmt.Errorf("bad value length: %d", vLen)
		return false
	}

	it.pendingValLen = vLen
	it.pendingValue = true
	return true
}

func (it *Iterator) ReadValue() ([]byte, bool) {
	if it.err != nil || it.eof {
		return nil, false
	}
	if !it.pendingValue {
		it.err = fmt.Errorf("ReadValue called without pending value")
		return nil, false
	}

	vLen := it.pendingValLen
	if cap(it.valBuf) < int(vLen) {
		it.valBuf = make([]byte, vLen)
	} else {
		it.valBuf = it.valBuf[:vLen]
	}

	if _, err := io.ReadFull(it.br, it.valBuf); err != nil {
		it.err = err
		return nil, false
	}

	it.pendingValue = false
	return it.valBuf, true
}

func (it *Iterator) SkipValue() bool {
	if it.err != nil || it.eof {
		return false
	}
	if !it.pendingValue {
		return true
	}

	n := int64(it.pendingValLen)
	_, err := io.CopyN(io.Discard, it.br, n)
	if err != nil {
		if err == io.EOF {
			it.eof = true
			return false
		}
		it.err = err
		return false
	}

	it.pendingValue = false
	return true
}
