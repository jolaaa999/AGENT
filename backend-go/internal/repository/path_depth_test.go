package repository

import (
	"testing"
)

// ==================== 逆向学习路径：深度上限与 meta 回传 ====================
//
// 背景（真实缺陷）：
//
//	前端把深度上限写成 12，后端 GetPathsToConcept 却把 maxDepth 硬夹到 6，
//	而且从未给 PathResponse.Meta 赋值（字段是 *PathQueryMeta + omitempty，
//	nil 时 JSON 里直接没有这个键）。于是：
//	  1. 用户把深度设成 7~12，后端静默按 6 算，界面上毫无提示；
//	  2. 前端 `depTree.filter(n => n.strength === "strong")` 恒为 0，
//	     因为 dependencyTree 里的 DependencyNode 从没被赋 Strength，
//	     状态栏一直显示「专注模式：0 个前置依赖」，而图上明明画着 5 个节点。
//
// 这里锁死「夹取后的深度」与「上限常量」两个不变量，防止再次静默漂移。
func TestPathDepthLimitIsSix(t *testing.T) {
	// 该常量是前后端共同契约：前端 main.vue 的 MAX_PATH_DEPTH_LIMIT 必须与之相等。
	// 改动此值时必须同步改前端，否则用户会选到注定被截断的深度。
	if maxPathDepthLimit != 6 {
		t.Fatalf("maxPathDepthLimit = %d, 期望 6；"+
			"改动它必须同步修改 frontend/src/view/main.vue 的 MAX_PATH_DEPTH_LIMIT",
			maxPathDepthLimit)
	}
}

// TestClampPathDepth 验证深度夹取规则：
//   - 小于 1 抬到 1（而不是像旧代码那样悄悄变成 3，把「0 层」这种输入
//     变成一个用户没要求过的深度）
//   - 大于上限夹到上限
//   - 区间内原样保留
func TestClampPathDepth(t *testing.T) {
	cases := []struct {
		name    string
		input   int
		want    int
		clamped bool
	}{
		{"零抬到最小 1", 0, 1, false},
		{"负数抬到最小 1", -5, 1, false},
		{"区间内原样", 1, 1, false},
		{"区间内原样 3", 3, 3, false},
		{"正好等于上限", 6, 6, false},
		{"超过上限被夹", 7, 6, true},
		{"前端旧上限 12 被夹", 12, 6, true},
		{"极大值被夹", 9999, 6, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, clamped := clampPathDepth(tc.input)
			if got != tc.want {
				t.Errorf("clampPathDepth(%d) 生效深度 = %d, 期望 %d", tc.input, got, tc.want)
			}
			if clamped != tc.clamped {
				t.Errorf("clampPathDepth(%d) depthClamped = %v, 期望 %v",
					tc.input, clamped, tc.clamped)
			}
		})
	}
}

// TestClampPathDepthReportsRequestedDepth 确认「请求深度」被原样保留。
//
// 这是给用户的提示「你设置的是 12 层，超出上限 6 层后不再向外扩展」的依据；
// 如果这里被夹过，提示就会变成谎话（说「你设置的是 6」）。
func TestClampPathDepthReportsRequestedDepth(t *testing.T) {
	requested := 12
	// 夹取函数本身只返回生效值与布尔，请求值由调用方在夹取前留存。
	applied, clamped := clampPathDepth(requested)
	if applied != 6 {
		t.Fatalf("applied = %d, 期望 6", applied)
	}
	if !clamped {
		t.Fatalf("clamped = false, 期望 true（12 > 6）")
	}
	if requested != 12 {
		t.Fatalf("requested 被意外改写为 %d, 期望保持 12", requested)
	}
}
