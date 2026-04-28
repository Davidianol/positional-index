package lsm

import (
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"
)

func newTestTree(t *testing.T) (*Tree, error) {
	dir := "./race_data"
	_ = os.RemoveAll(dir)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return New(dir)
}

// Одновременные Put / Get / Delete поверх одного дерева
func TestTreeConcurrentAccess(t *testing.T) {
	tree, _ := newTestTree(t)

	const (
		writerGoroutines  = 4
		readerGoroutines  = 4
		deleterGoroutines = 2
		opsPerGoroutine   = 10_000
	)

	var wg sync.WaitGroup
	wg.Add(writerGoroutines + readerGoroutines + deleterGoroutines)

	rand.Seed(time.Now().UnixNano())

	// постоянно пишут случайные ключи
	for w := 0; w < writerGoroutines; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				k := fmt.Sprintf("k_w%d_%08d", id, rand.Intn(100_000))
				v := fmt.Sprintf("v_%08d", i)
				tree.Put(k, v)
			}
		}(w)
	}

	// читают случайные ключи (в основном существующие)
	for r := 0; r < readerGoroutines; r++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				k := fmt.Sprintf("k_w%d_%08d", rand.Intn(writerGoroutines), rand.Intn(100_000))
				_, _ = tree.Get(k)
			}
		}(r)
	}

	// периодически ставят tombstone
	for d := 0; d < deleterGoroutines; d++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				k := fmt.Sprintf("k_w%d_%08d", rand.Intn(writerGoroutines), rand.Intn(100_000))
				tree.Delete(k)
			}
		}(d)
	}

	wg.Wait()
}
