package events

type Deduper struct {
	seen map[string]struct{}
}

func NewDeduper() *Deduper {
	return &Deduper{seen: make(map[string]struct{})}
}

func (d *Deduper) MarkIfNew(event NormalizedEvent) bool {
	if d == nil {
		return false
	}
	if _, ok := d.seen[event.DeliveryKey]; ok {
		return false
	}
	d.seen[event.DeliveryKey] = struct{}{}
	return true
}
