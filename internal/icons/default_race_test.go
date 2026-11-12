package icons

import (
	"fmt"
	"sync"
	"testing"
)

// Default is called from several goroutines at once (builds, metadata
// fetches, Discover tiles); it must not corrupt the shared font face.
func TestDefaultConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if len(Default(fmt.Sprintf("App %c", 'A'+i))) == 0 {
				t.Error("empty icon")
			}
		}(i)
	}
	wg.Wait()
}
