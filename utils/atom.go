package utils

import "sync/atomic"

// AtomicBool 封装了int32类型的原子操作
type AtomicBool struct {
	flag int32
}

// Load 读取当前的布尔值
func (a *AtomicBool) Load() bool {
	return atomic.LoadInt32(&a.flag) == 1
}

// Store 设置布尔值
func (a *AtomicBool) Store(val bool) {
	var v int32
	if val {
		v = 1
	}
	atomic.StoreInt32(&a.flag, v)
}

// CompareAndSwap 比较并交换布尔值
func (a *AtomicBool) CompareAndSwap(oldVal, newVal bool) bool {
	// 将布尔值转换为 int32
	oldInt32 := int32(1) // 假设 oldVal 为 true
	newInt32 := int32(0) // 假设 newVal 为 false

	if !oldVal {
		oldInt32 = 0
	}
	if !newVal {
		newInt32 = 0
	}

	// 使用 atomic.CompareAndSwapInt32 进行原子比较并交换
	return atomic.CompareAndSwapInt32(&a.flag, oldInt32, newInt32)
}
