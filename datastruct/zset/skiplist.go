package zset

import "math/rand"

// 对外的元素抽象
type Element struct {
	Member string
	Score  float64
}

type Node struct {
	Element
	backward *Node //后向指针
	level    []*Level
}

// 节点中每一层的抽象
type Level struct {
	forward *Node
	span    int64 // 到 forward 跳过的节点数 (跨度)
}
type skiplist struct {
	header *Node
	tail   *Node
	length int64
	level  int16 // 	高度
}

const (
	maxLevel = 16
)

func makeNode(level int16, score float64, member string) *Node {
	n := &Node{
		Element: Element{
			Score:  score,
			Member: member,
		},
		level: make([]*Level, level),
	}
	for i := range n.level {
		n.level[i] = new(Level)
	}
	return n
}

// 寻找排名为 rank 的节点, rank 从1开始
func (skiplist *skiplist) getByRank(rank int64) *Node {
	var i int64 = 0 //累计跨度
	n := skiplist.header
	// 自顶向下查询
	for curLevel := skiplist.level - 1; curLevel >= 0; curLevel-- {
		// 从当前层向前搜索，若当前层下一个节点已经超过目标(i+n.level[level].span > rank)，则结束当前搜索进入下一层
		for n.level[curLevel].forward != nil && (i+n.level[curLevel].span) <= rank {
			i += n.level[curLevel].span
			n = n.level[curLevel].forward //移动到当前层下一个节点
		}
		if i == rank {
			return n
		}
	}
	return nil
}

func (skiplist *skiplist) hasInRange(min Border, max Border) bool {
	if min.isIntersected(max) { //是有交集的，则返回false
		return false
	}

	// min > tail
	n := skiplist.tail
	if n == nil || !min.less(&n.Element) {
		return false
	}
	// max < head
	n = skiplist.header.level[0].forward
	if n == nil || !max.greater(&n.Element) {
		return false
	}
	return true
}

// ZRangeByScore 命令需要 getFirstInScoreRange 函数找到分数范围内第一个节点:
func (skiplist *skiplist) getFirstInScoreRange(min Border, max Border) *Node {
	// 判断跳表和范围是否有交集，若无交集提早返回
	if !skiplist.hasInRange(min, max) {
		return nil
	}
	n := skiplist.header
	// 从顶层向下查询
	for level := skiplist.level - 1; level >= 0; level-- {
		// 若 forward 节点仍未进入范围则继续向前(forward)
		// 若 forward 节点已进入范围，当 level > 0 时 forward 节点不能保证是 *第一个* 在 min 范围内的节点， 因此需进入下一层查找
		for n.level[level].forward != nil && !min.less(&n.level[level].forward.Element) {
			n = n.level[level].forward
		}
	}
	// 当从外层循环退出时 level=0 (最下层), n.level[0].forward 一定是 min 范围内的第一个节点
	n = n.level[0].forward
	if !max.greater(&n.Element) {
		return nil
	}
	return n
}

// insert

func (skiplist *skiplist) insert(member string, score float64) *Node {
	// 寻找新节点的先驱节点，它们的 forward 将指向新节点
	// 因为每层都有一个 forward 指针, 所以每层都会对应一个先驱节点
	// 找到这些先驱节点并保存在 update 数组中
	update := make([]*Node, maxLevel)
	rank := make([]int64, maxLevel) // 保存各层先驱节点的排名，用于计算span
	node := skiplist.header
	for i := skiplist.level - 1; i >= 0; i-- {
		// 初始化 rank
		if i == skiplist.level-1 {
			rank[i] = 0
		} else {
			rank[i] = rank[i+1]
		}
		if node.level[i] != nil {
			// 遍历搜索
			for node.level[i].forward != nil &&
				(node.level[i].forward.Score < score ||
					(node.level[i].forward.Score == score && node.level[i].forward.Member < member)) { // same score, different key
				rank[i] += node.level[i].span
				// 当前节点移动到下一个
				node = node.level[i].forward
			}
		}
		// 在第i层找到一个，加入数组
		update[i] = node
	}
	level := randomLevel() // 随机决定新节点的层数
	// 创建新的层
	if level > skiplist.level {
		for i := skiplist.level; i < level; i++ {
			rank[i] = 0
			update[i] = skiplist.header
			update[i].level[i].span = skiplist.length
		}
		skiplist.level = level
	}
	// 创建新节点并插入跳表
	newNode := makeNode(level, score, member)
	for i := int16(0); i < level; i++ {
		// 新节点的 forward 指向先驱节点的 forward
		newNode.level[i].forward = update[i].level[i].forward
		//
		update[i].level[i].forward = newNode
		// 计算先驱节点和新节点的 span
		node.level[i].span = update[i].level[i].span - (rank[0] - rank[i])
		update[i].level[i].span = (rank[0] - rank[i]) + 1
	}
	// 新节点可能不会包含所有层
	// 对于没有层，先驱节点的 span 会加1 (后面插入了新节点导致span+1)
	for i := level; i < skiplist.level; i++ {
		update[i].level[i].span++
	}
	// 更新后向指针
	if update[0] == skiplist.header {
		node.backward = nil
	} else {
		node.backward = update[0]
	}
	if node.level[0].forward != nil {
		node.level[0].forward.backward = node
	} else {
		skiplist.tail = node
	}
	skiplist.length++
	return node
}
func randomLevel() int16 {
	level := int16(1)
	for float32(rand.Int31()&0xFFFF) < (0.25 * 0xFFFF) {
		level++
	}
	if level < maxLevel {
		return level
	}
	return maxLevel
}

// 删除操作可能一次删除多个节点
func (skiplist *skiplist) RemoveRangeByRank(start int64, stop int64) (removed []*Element) {
	var i int64 = 0 // 当前指针的排名
	update := make([]*Node, maxLevel)
	removed = make([]*Element, 0)

	// 从顶层向下寻找目标的先驱节点
	node := skiplist.header
	for level := skiplist.level - 1; level >= 0; level-- {
		for node.level[level].forward != nil && (i+node.level[level].span) < start {
			i += node.level[level].span
			node = node.level[level].forward
		}
		update[level] = node
	}

	i++
	node = node.level[0].forward // node 是目标范围内第一个节点

	// 删除范围内的所有节点
	for node != nil && i < stop {
		next := node.level[0].forward
		removedElement := node.Element
		removed = append(removed, &removedElement)
		skiplist.removeNode(node, update)
		node = next
		i++
	}
	return removed
}

// 传入目标节点和删除后的先驱节点
// 在批量删除时我们传入的 update 数组是相同的
func (skiplist *skiplist) removeNode(node *Node, update []*Node) {
	for i := int16(0); i < skiplist.level; i++ {
		// 如果先驱节点的forward指针指向了目标节点，则需要修改先驱的forward指针跳过要删除的目标节点
		// 同时更新先驱的 span
		if update[i].level[i].forward == node {
			update[i].level[i].span += node.level[i].span - 1
			update[i].level[i].forward = node.level[i].forward
		} else {
			update[i].level[i].span--
		}
	}
	// 修改目标节点后继节点的backward指针
	if node.level[0].forward != nil {
		node.level[0].forward.backward = node.backward
	} else {
		skiplist.tail = node.backward
	}
	// 必要时删除空白的层
	for skiplist.level > 1 && skiplist.header.level[skiplist.level-1].forward == nil {
		skiplist.level--
	}
	skiplist.length--
}
