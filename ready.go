package traefik

func (t *Traefik) Ready() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.ready
}
