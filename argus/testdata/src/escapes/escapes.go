// Package escapes is read by the model tests, not by a rule: which results of a
// method hand out the receiver's own state. A return inside a func literal is the
// literal's result, whether the literal runs in place or is handed out.
package escapes

type Cache struct {
	items []string
	index map[string]int
}

func (c *Cache) Items() []string { return c.items }

func (c *Cache) Count() int {
	snapshot := func() []string { return c.items }
	return len(snapshot())
}

func (c *Cache) Lookup() func(string) int {
	return func(key string) int { return c.index[key] }
}

func (c *Cache) Visit(visit func(string) bool) map[string]int {
	for _, item := range c.items {
		if visit(item) == false {
			return nil
		}
	}
	return c.index
}
