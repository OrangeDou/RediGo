package zset

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

// ZRangeByScore 命令需要 getFirstInScoreRange 函数找到分数范围内第一个节点:
func (skiplist *skiplist) getFirstInScoreRange(min *ScoreBorder, max *ScoreBorder) *Node {
	// 判断跳表和范围是否有交集，若无交集提早返回
	if !skiplist.hasInRange(min, max) {
		return nil
	}
	n := skiplist.header
	// 从顶层向下查询
	for level := skiplist.level - 1; level >= 0; level-- {
		// 若 forward 节点仍未进入范围则继续向前(forward)
		// 若 forward 节点已进入范围，当 level > 0 时 forward 节点不能保证是 *第一个* 在 min 范围内的节点， 因此需进入下一层查找
		for n.level[level].forward != nil && !min.less(n.level[level].forward.Score) {
			n = n.level[level].forward
		}
	}
	// 当从外层循环退出时 level=0 (最下层), n.level[0].forward 一定是 min 范围内的第一个节点
	n = n.level[0].forward
	if !max.greater(n.Score) {
		return nil
	}
	return n
}
