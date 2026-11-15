package mobile

import (
	"sync"
	"testing"
)

func TestScannerBeginEndOverlapPreservesNewerCancel(t *testing.T) {
	s := NewScanner()
	ctx1, cancel1, seq1 := s.begin()
	ctx2, cancel2, seq2 := s.begin()

	// El primer escaneo debería quedar cancelado al empezar el segundo.
	select {
	case <-ctx1.Done():
	default:
		t.Fatal("ctx1 debería haber sido cancelado al iniciar el segundo begin()")
	}

	// Cuando termina el primero, NO debe borrar el cancel del segundo.
	s.end(seq1, cancel1)

	select {
	case <-ctx2.Done():
		t.Fatal("s.end del primer scan canceló erróneamente ctx2")
	default:
	}

	// Cancel() aún debe cancelar ctx2.
	s.Cancel()
	select {
	case <-ctx2.Done():
	default:
		t.Fatal("s.Cancel() no canceló ctx2 después de s.end(seq1)")
	}

	s.end(seq2, cancel2)
}

func TestScannerBeginEndConcurrent(t *testing.T) {
	s := NewScanner()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, cancel, seq := s.begin()
			s.Cancel()
			s.end(seq, cancel)
		}()
	}
	wg.Wait()
}
