package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"inverse_index/internal/lsm"
)

const (
	dataDir    = "./bench_data_cmd"
	itemCount  = 100_000
	rangeWidth = 100
)

func generateKV(i int) (string, string) {
	return fmt.Sprintf("key_%08d", i), fmt.Sprintf("value_payload_%08d", i)
}

func benchInsert(b *testing.B) {
	_ = os.RemoveAll(dataDir)
	tree, err := lsm.New(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		k, v := generateKV(i)
		tree.Put(k, v)
	}
}

func benchGet(b *testing.B) {
	_ = os.RemoveAll(dataDir)
	tree, err := lsm.New(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	for i := 0; i < itemCount; i++ {
		k, v := generateKV(i)
		tree.Put(k, v)
	}

	time.Sleep(500 * time.Millisecond)

	rand.Seed(1)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id := rand.Intn(itemCount)
		key, _ := generateKV(id)
		tree.Get(key)
	}
}

func benchRange(b *testing.B) {
	_ = os.RemoveAll(dataDir)
	tree, err := lsm.New(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	for i := 0; i < itemCount; i++ {
		k, v := generateKV(i)
		tree.Put(k, v)
	}

	time.Sleep(500 * time.Millisecond)

	rand.Seed(2)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		startIdx := rand.Intn(itemCount - rangeWidth)
		startKey, _ := generateKV(startIdx)
		endKey, _ := generateKV(startIdx + rangeWidth)
		_, _ = tree.RangeScan(startKey, endKey, rangeWidth)
	}
}

func main() {
	fmt.Println("=== Insert (sequential) ===")
	resInsert := testing.Benchmark(benchInsert)
	fmt.Println(resInsert.String(), resInsert.MemString())

	fmt.Println("\n=== Get (random) ===")
	resGet := testing.Benchmark(benchGet)
	fmt.Println(resGet.String(), resGet.MemString())

	fmt.Println("\n=== RangeScan (100 keys) ===")
	resRange := testing.Benchmark(benchRange)
	fmt.Println(resRange.String(), resRange.MemString())
}
