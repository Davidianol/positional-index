package main

import (
	"fmt"
	"log"
	"os"

	"inverse_index/internal/index"
)

var corpus = []struct {
	id   uint32
	text string
}{
	// --- Go ---
	{1, "Go is an open source programming language that makes it easy to build reliable software"},
	{4, "Go compiler produces efficient machine code and the runtime provides garbage collection"},
	{7, "Go routines and channels make concurrent programming simple and efficient"},
	{11, "Go interfaces allow duck typing and make code highly composable and testable"},
	{12, "Go modules provide dependency management and reproducible builds for projects"},
	{13, "Go standard library includes powerful packages for networking http json and cryptography"},
	{14, "Go generics introduced in version 1.18 allow writing reusable type safe data structures"},
	{15, "Go memory model defines how goroutines communicate through shared memory and channels"},
	{16, "Go profiling tools like pprof help identify CPU and memory bottlenecks in applications"},

	// --- LSM Trees ---
	{3, "LSM trees are used in databases like RocksDB LevelDB and Cassandra for fast writes"},
	{8, "Compaction in LSM trees merges SSTables to reduce read amplification and reclaim space"},
	{21, "LSM tree memtable holds recent writes in memory before flushing to disk as SSTables"},
	{22, "Leveled compaction strategy keeps LSM tree compact but increases write amplification"},
	{23, "Tiered compaction reduces write amplification in LSM trees at the cost of read performance"},
	{24, "LSM trees separate read and write paths making writes extremely fast and sequential"},
	{25, "WAL write ahead log ensures durability in LSM trees before data reaches the memtable"},
	{27, "LSM tree compaction removes tombstones and merges duplicate keys into single values"},
	{45, "Full text search engines like Elasticsearch use segment based structures similar to LSM trees"},
	{63, "RocksDB is a persistent key value store based on LSM tree optimized for fast storage devices"},
	{64, "Cassandra uses LSM tree structure to achieve high write throughput on distributed clusters"},
	{67, "Read amplification in LSM tree increases with number of levels requiring multi component merge"},

	// --- Roaring Bitmaps ---
	{2, "Roaring bitmaps are compressed bitmaps which outperform conventional bitsets"},
	{31, "Roaring bitmap divides uint32 space into chunks of 65536 values identified by high 16 bits"},
	{32, "Array container in roaring bitmap stores sparse data as sorted list of uint16 values"},
	{33, "Bitmap container in roaring bitmap stores dense data as 1024 uint64 words using 8 kilobytes"},
	{34, "Roaring bitmap automatically switches between array and bitmap container at 4096 element threshold"},
	{35, "Roaring bitmap intersection uses bitwise AND over 1024 word bitmap containers efficiently"},
	{36, "Roaring bitmap union uses bitwise OR and is the core operation in inverted index merging"},
	{37, "Rank and select operations on roaring bitmaps enable compact prefix tree navigation"},
	{38, "Roaring bitmap serialization stores container type key and data for efficient persistence"},

	// --- Databases General ---
	{61, "B-tree is the dominant data structure in relational databases and file systems for range queries"},
	{62, "B-tree provides logarithmic point lookup and efficient range scan in a single unified tree"},
	{66, "Write amplification measures how many bytes are written to disk per byte of user data"},
	{68, "Space amplification refers to extra disk space consumed by duplicate and obsolete data"},
	{69, "Database index trades storage space and write overhead for faster query execution time"},
	{70, "Column oriented databases store data by column enabling efficient aggregation and compression"},

	// --- Systems & Performance ---
	{101, "Sequential IO on SSD is orders of magnitude faster than random IO motivating LSM design"},
	{102, "Memory mapped files allow OS to handle caching and avoid explicit read syscalls overhead"},
	{103, "Write ahead logging ensures crash recovery by persisting intent before applying changes"},
	{104, "Lock free data structures use atomic compare and swap to avoid mutex contention overhead"},
	{105, "Cache locality matters significantly in performance sensitive code for CPU cache utilization"},
	{106, "Buffered IO groups small writes into larger sequential blocks improving SSD throughput"},
	{107, "Compaction bandwidth limits how fast inverted index can be built on high write workloads"},
	{108, "Hot storage provides high IOPS for recent frequently accessed data at higher cost per byte"},
	{109, "Cold storage archives rarely accessed historical data at significantly lower cost per byte"},
	{110, "Partitioning index by document age allows recent partition to stay small and fast to update"},
}

func main() {
	dir := "./index_data"
	_ = os.RemoveAll(dir)
	defer os.RemoveAll(dir)

	idx, err := index.NewInvertedIndex(dir, "english")
	if err != nil {
		log.Fatal(err)
	}

	for _, doc := range corpus {
		idx.Index(doc.id, doc.text)
	}
	fmt.Printf("Indexed %d docs\n\n", len(corpus))

	run := func(label string, expr index.Expr) {
		bm, err := idx.Query(expr)
		if err != nil {
			fmt.Printf("[%s] ERR: %v\n", label, err)
			return
		}
		fmt.Printf("[%s]\n  docs=%v  count=%d\n\n",
			label, bm.ToArray(), bm.GetCardinality())
	}

	run(`Term("go")`,
		index.Term("go"))

	run(`"lsm" AND "tree"`,
		index.And(index.Term("lsm"), index.Term("tree")))

	run(`"bitmap" OR "bloom"`,
		index.Or(index.Term("bitmap"), index.Term("bloom")))

	run(`NOT "go"  (все документы без go)`,
		index.Not(index.Term("go")))

	run(`("lsm" AND "compact") OR ("go" AND "concurr")`,
		index.Or(
			index.And(index.Term("lsm"), index.Term("compact")),
			index.And(index.Term("go"), index.Term("concurrent")),
		))
}
