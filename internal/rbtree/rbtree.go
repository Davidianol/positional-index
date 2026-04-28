package rbtree

const (
	red   = true
	black = false
)

type Node struct {
	Key   string
	Value string
	color bool
	left  *Node
	right *Node
}

func isRed(n *Node) bool {
	return n != nil && n.color == red
}
func rotateLeft(h *Node) *Node {
	x := h.right
	h.right = x.left
	x.left = h
	x.color = h.color
	h.color = red
	return x
}

func rotateRight(h *Node) *Node {
	x := h.left
	h.left = x.right
	x.right = h
	x.color = h.color
	h.color = red
	return x
}

func flipColors(h *Node) {
	h.color = !h.color
	h.left.color = !h.left.color
	h.right.color = !h.right.color
}

type RBTree struct {
	root  *Node
	count int
	size  int // суммарный len(key)+len(value)
}

func New() *RBTree { return &RBTree{} }

func (t *RBTree) Count() int { return t.count }
func (t *RBTree) Size() int  { return t.size }

func (t *RBTree) Put(key, value string) {
	var added bool
	var delta int
	t.root, added, delta = put(t.root, key, value)
	t.root.color = black
	if added {
		t.count++
	}
	t.size += delta
}

func put(h *Node, key, value string) (node *Node, added bool, delta int) {
	if h == nil {
		return &Node{Key: key, Value: value, color: red}, true, len(key) + len(value)
	}

	switch {
	case key < h.Key:
		h.left, added, delta = put(h.left, key, value)
	case key > h.Key:
		h.right, added, delta = put(h.right, key, value)
	default:
		delta = len(value) - len(h.Value)
		h.Value = value
		return h, false, delta
	}

	if isRed(h.right) && !isRed(h.left) {
		h = rotateLeft(h)
	}

	if isRed(h.left) && isRed(h.left.left) {
		h = rotateRight(h)
	}
	if isRed(h.left) && isRed(h.right) {
		flipColors(h)
	}

	return h, added, delta
}

func (t *RBTree) Get(key string) (string, bool) {
	n := t.root
	for n != nil {
		switch {
		case key < n.Key:
			n = n.left
		case key > n.Key:
			n = n.right
		default:
			return n.Value, true
		}
	}
	return "", false
}

func (t *RBTree) InOrder(fn func(key, value string)) {
	inOrder(t.root, fn)
}

func inOrder(n *Node, fn func(key, value string)) {
	if n == nil {
		return
	}
	inOrder(n.left, fn)
	fn(n.Key, n.Value)
	inOrder(n.right, fn)
}

func (t *RBTree) Min() string {
	if t.root == nil {
		return ""
	}
	n := t.root
	for n.left != nil {
		n = n.left
	}
	return n.Key
}

func (t *RBTree) Max() string {
	if t.root == nil {
		return ""
	}
	n := t.root
	for n.right != nil {
		n = n.right
	}
	return n.Key
}
