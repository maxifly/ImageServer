package opermanager

import "sync"

// IdMutex обеспечивает потокобезопасный доступ к данным по id
type IdMutex struct {
	mu    sync.RWMutex
	locks map[string]*sync.Mutex
	refs  map[string]int
}

func NewIdMutex() *IdMutex {
	return &IdMutex{
		locks: make(map[string]*sync.Mutex),
		refs:  make(map[string]int),
	}
}

// GetLock возвращает мьютекс для данного id и увеличивает счетчик ссылок
func (s *IdMutex) GetLock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.locks[id]; !exists {
		s.locks[id] = &sync.Mutex{}
	}
	s.refs[id]++
	return s.locks[id]
}

// ReleaseLock уменьшает счетчик ссылок и удаляет мьютекс, если счетчик равен нулю
func (s *IdMutex) ReleaseLock(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if count, exists := s.refs[id]; exists {
		count--
		s.refs[id] = count
		if count == 0 {
			delete(s.locks, id)
			delete(s.refs, id)
		}
	}
}
