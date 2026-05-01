package index

import "encoding/binary"

// PostingList: docID -> sorted list of in-document positions.
type PostingList map[uint32][]uint32

func (pl PostingList) Encode() []byte {
	size := 4
	for _, positions := range pl {
		size += 4 + 4 + 4*len(positions)
	}
	buf := make([]byte, size)
	off := 0
	binary.BigEndian.PutUint32(buf[off:], uint32(len(pl)))
	off += 4
	for docID, positions := range pl {
		binary.BigEndian.PutUint32(buf[off:], docID)
		off += 4
		binary.BigEndian.PutUint32(buf[off:], uint32(len(positions)))
		off += 4
		for _, p := range positions {
			binary.BigEndian.PutUint32(buf[off:], p)
			off += 4
		}
	}
	return buf
}

func DecodePostingList(data []byte) (PostingList, error) {
	if len(data) < 4 {
		return PostingList{}, nil
	}
	pl := PostingList{}
	off := 0
	n := binary.BigEndian.Uint32(data[off:])
	off += 4
	for i := uint32(0); i < n; i++ {
		if off+8 > len(data) {
			break
		}
		docID := binary.BigEndian.Uint32(data[off:])
		off += 4
		m := binary.BigEndian.Uint32(data[off:])
		off += 4
		positions := make([]uint32, m)
		for j := uint32(0); j < m; j++ {
			if off+4 > len(data) {
				break
			}
			positions[j] = binary.BigEndian.Uint32(data[off:])
			off += 4
		}
		pl[docID] = positions
	}
	return pl, nil
}

func MergePostingLists(a, b PostingList) PostingList {
	out := make(PostingList, len(a))
	for docID, positions := range a {
		cp := make([]uint32, len(positions))
		copy(cp, positions)
		out[docID] = cp
	}
	for docID, positions := range b {
		if existing, ok := out[docID]; ok {
			out[docID] = mergeSortedUint32(existing, positions)
		} else {
			cp := make([]uint32, len(positions))
			copy(cp, positions)
			out[docID] = cp
		}
	}
	return out
}

func mergeSortedUint32(a, b []uint32) []uint32 {
	out := make([]uint32, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			out = append(out, a[i])
			i++
		case a[i] > b[j]:
			out = append(out, b[j])
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	out = append(out, a[i:]...)
	out = append(out, b[j:]...)
	return out
}
