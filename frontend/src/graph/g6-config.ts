import type { GraphEdge, GraphNode, GraphResponse } from "../api/graph";

/**
 * 获取边路由配置（使用 polyline 类型配合控制点）
 * 通过自定义路由让边绕开节点
 */
export function getEdgeRoutingConfig() {
  return {
    type: "polyline",
    style: {
      router: {
        type: "orth",  // 正交路由
        padding: 10    // 绕开节点的边距
      },
      curveOffset: 20,
      endArrow: true
    }
  };
}

export interface StyledGraphData {
  nodes: Array<Record<string, any>>;
  edges: Array<Record<string, any>>;
}

export interface PreprocessOptions {
  minConfidence?: number;
  removeSelfLoops?: boolean;
  keepIsolatedNodes?: boolean;
}

/**
 * 图谱数据预处理：合并同义节点、过滤低质量边、降低噪声
 */
export function preprocessGraphData(
  nodes: GraphNode[],
  edges: GraphEdge[],
  options: PreprocessOptions = {}
): GraphResponse {
  const { minConfidence = 0.6, removeSelfLoops = true, keepIsolatedNodes = false } = options;

  console.log("[预处理] 原始数据:", { nodes: nodes.length, edges: edges.length });

  // 1. 实体消歧：基于概念名称的归一化合并
  // nameMap: normalized label -> 保留的 node.id（首次出现的节点）
  const nameMap = new Map<string, string>();
  // idMapping: node.id -> 保留的 node.id（用于边映射和节点去重）
  const idMapping = new Map<string, string>();

  nodes.forEach((node) => {
    const normalized = node.label.toLowerCase().trim();
    if (!nameMap.has(normalized)) {
      nameMap.set(normalized, node.id);
    }
    // 所有相同 label 的节点都映射到同一个保留的 node.id
    const canonicalId = nameMap.get(normalized)!;
    idMapping.set(node.id, canonicalId);
  });

  console.log("[预处理] nameMap 大小:", nameMap.size, "idMapping 大小:", idMapping.size);

  // 检查是否有重复 label 的节点
  const duplicateLabels = new Map<string, string[]>();
  nodes.forEach((node) => {
    const normalized = node.label.toLowerCase().trim();
    if (!duplicateLabels.has(normalized)) {
      duplicateLabels.set(normalized, []);
    }
    duplicateLabels.get(normalized)!.push(node.id);
  });
  const duplicates = Array.from(duplicateLabels.entries()).filter(([_, ids]) => ids.length > 1);
  if (duplicates.length > 0) {
    console.log("[预处理] 发现重复 label 的节点:", duplicates.map(([label, ids]) => ({ label, ids })));
  }

  // 2. 关系过滤：移除低置信度边和自环，返回false去掉
  const filteredEdges = edges.filter((edge) => {
    const confidenceOk = (edge.confidence ?? 1.0) >= minConfidence;
    const notSelfLoop = !removeSelfLoops || edge.source !== edge.target;
    return confidenceOk && notSelfLoop;
  });

  console.log("[预处理] 过滤后边数:", filteredEdges.length, "(移除", edges.length - filteredEdges.length, ")");

  // 3. 映射边的 source/target 到归一化后的节点 ID
  const remappedEdges = filteredEdges.map((edge) => ({
    ...edge,
    source: idMapping.get(edge.source) || edge.source,
    target: idMapping.get(edge.target) || edge.target
  }));

  // 检查映射后的边是否有问题
  const invalidEdges = remappedEdges.filter(e => e.source === e.target);
  if (invalidEdges.length > 0) {
    console.log("[预处理] 警告: 发现自环边:", invalidEdges);
  }

  // 4. 去重边（相同 source-target-label 的边只保留一条，保留最高置信度的）
  const edgeMap = new Map<string, GraphEdge>();
  remappedEdges.forEach((edge) => {
    const key = `${edge.source}-${edge.target}-${edge.label}`;
    const existing = edgeMap.get(key);
    if (!existing || (edge.confidence ?? 1.0) > (existing.confidence ?? 1.0)) {
      edgeMap.set(key, edge);
    }
  });
  const deduplicatedEdges = Array.from(edgeMap.values());

  console.log("[预处理] 去重后边数:", deduplicatedEdges.length, "(移除", remappedEdges.length - deduplicatedEdges.length, ")");

  // 5. 仅保留有关联的节点（或根据配置保留孤立节点）
  const connectedNodeIds = new Set<string>();
  deduplicatedEdges.forEach((e) => {
    connectedNodeIds.add(e.source);
    connectedNodeIds.add(e.target);
  });

  console.log("[预处理] 有关联的节点数:", connectedNodeIds.size);

  const filteredNodes = keepIsolatedNodes
    ? nodes.map((n) => ({ ...n, id: idMapping.get(n.id) || n.id }))
    : nodes
        .filter((n) => connectedNodeIds.has(idMapping.get(n.id) || n.id))
        .map((n) => ({ ...n, id: idMapping.get(n.id) || n.id }));

  // 6. 节点去重（基于归一化后的 ID）
  const nodeMap = new Map<string, GraphNode>();

  filteredNodes.forEach((node) => {
    const normalizedId = idMapping.get(node.id) || node.id;
    if (!nodeMap.has(normalizedId)) {
      nodeMap.set(normalizedId, { ...node, id: normalizedId });
    }
  });

  const result = {
    nodes: Array.from(nodeMap.values()),
    edges: deduplicatedEdges
  };

  console.log("[预处理] 最终结果:", { nodes: result.nodes.length, edges: result.edges.length });

  // 检查是否有重复 ID 的节点（这会导致 G6 渲染问题）
  const idCounts = new Map<string, number>();
  result.nodes.forEach((n) => {
    idCounts.set(n.id, (idCounts.get(n.id) || 0) + 1);
  });
  const duplicateIds = Array.from(idCounts.entries()).filter(([_, count]) => count > 1);
  if (duplicateIds.length > 0) {
    console.error("[预处理] 错误: 发现重复 ID 的节点:", duplicateIds);
  }

  return result;
}

const nodeStatusStyle: Record<
  string,
  { fill: string; stroke: string; lineWidth: number; lineDash?: number[] }
> = {
  correct: {
    fill: "#DBEAFE",
    stroke: "#2563EB",
    lineWidth: 1.5
  },
  error: {
    fill: "#FEE2E2",
    stroke: "#DC2626",
    lineWidth: 3
  },
  supplement: {
    fill: "#F3E8FF",
    stroke: "#7C3AED",
    lineWidth: 2.5,
    lineDash: [8, 6]
  }
};

function truncateLabel(label: string, maxLength = 12) {
  const normalized = label.trim();
  return normalized.length > maxLength ? `${normalized.slice(0, maxLength)}…` : normalized;
}

function styleNode(node: GraphNode, dimmed: boolean, degree: number) {
  const preset = nodeStatusStyle[node.status ?? ""] ?? {
    fill: "#E2E8F0",
    stroke: "#64748B",
    lineWidth: 1.5
  };
  const size = Math.min(64, 38 + Math.sqrt(Math.max(1, degree)) * 5);

  const safeData: Record<string, any> = { ...node };
  if (typeof safeData.type === "number") {
    safeData.nodeType = String(safeData.type);
    delete safeData.type;
  }

  // 如果节点有预计算位置（如扇形展开），保留它
  const result: any = {
    id: String(node.id),
    data: safeData,
    style: {
      size,
      labelText: truncateLabel(node.label || node.id),
      labelPlacement: "bottom",
      labelOffsetY: 7,
      fill: preset.fill,
      stroke: preset.stroke,
      lineWidth: degree >= 6 ? Math.max(2.5, preset.lineWidth) : preset.lineWidth,
      lineDash: preset.lineDash,
      opacity: dimmed ? 0.12 : 1,
      labelFill: "#1E293B",
      labelFontSize: degree >= 6 ? 12 : 11,
      labelFontWeight: degree >= 6 ? 600 : 400,
      halo: degree >= 6,
      haloLineWidth: 8,
      haloStroke: preset.fill
    }
  };

  // 保留预计算的 x, y 位置
  if ((node as any).x !== undefined) {
    result.x = (node as any).x;
  }
  if ((node as any).y !== undefined) {
    result.y = (node as any).y;
  }

  return result;
}

function styleEdge(edge: GraphEdge, dimmed: boolean) {
  const isError = edge.status === "error";
  const isSupplement = edge.status === "supplement";

  const safeData: Record<string, any> = { ...edge };
  if (typeof safeData.type === "number") {
    safeData.edgeType = String(safeData.type);
    delete safeData.type;
  }

  return {
    id: String(edge.id || `${edge.source}-${edge.target}-${edge.label}`),
    source: String(edge.source),
    target: String(edge.target),
    data: safeData,
    style: {
      labelText: truncateLabel(edge.label || "关联", 16),
      labelPlacement: "center",
      labelFill: isError ? "#B91C1C" : isSupplement ? "#6D28D9" : "#64748B",
      labelFontSize: 9,
      labelBackground: true,
      labelBackgroundFill: "#FFFFFF",
      labelBackgroundFillOpacity: 0.86,
      labelBackgroundRadius: 3,
      labelBackgroundPadding: [1, 3],
      stroke: isError ? "#DC2626" : isSupplement ? "#7C3AED" : "#CBD5E1",
      lineDash: isSupplement ? [6, 5] : undefined,
      lineWidth: isError ? 2 : 1,
      endArrow: true,
      endArrowSize: 5,
      opacity: dimmed ? 0.06 : isError || isSupplement ? 0.75 : 0.55
    }
  };
}

export function buildStyledGraph(
  graph: GraphResponse,
  focus?: { nodeIds: Set<string>; edgeIds: Set<string> }
): StyledGraphData {
  const hasFocus = Boolean(focus && (focus.nodeIds.size > 0 || focus.edgeIds.size > 0));
  const degrees = new Map<string, number>();
  graph.nodes.forEach((node) => degrees.set(node.id, 0));
  graph.edges.forEach((edge) => {
    degrees.set(edge.source, (degrees.get(edge.source) ?? 0) + 1);
    degrees.set(edge.target, (degrees.get(edge.target) ?? 0) + 1);
  });

  const nodes = graph.nodes.map((node) =>
    styleNode(node, hasFocus ? !focus!.nodeIds.has(node.id) : false, degrees.get(node.id) ?? 0)
  );
  const edges = graph.edges.map((edge) => {
    const edgeId = edge.id || `${edge.source}-${edge.target}-${edge.label}`;
    return styleEdge(edge, hasFocus ? !focus!.edgeIds.has(edgeId) : false);
  });

  return { nodes, edges };
}

export function buildFocusSet(paths: GraphResponse[]): { nodeIds: Set<string>; edgeIds: Set<string> } {
  const nodeIds = new Set<string>();
  const edgeIds = new Set<string>();

  paths.forEach((path) => {
    path.nodes.forEach((node) => nodeIds.add(node.id));
    path.edges.forEach((edge) => edgeIds.add(edge.id || `${edge.source}-${edge.target}-${edge.label}`));
  });

  return { nodeIds, edgeIds };
}

/**
 * 布局类型
 */
export type LayoutType = "force" | "dagre";

/**
 * 获取力导向布局配置（普通浏览模式）
 * 优化参数：防止重叠、合适的斥力、边长度
 * G6 v5 中 preventOverlap 是软约束，通过增大 nodeSpacing 和 collideStrength 来增强防重叠效果
 */
export function getForceLayoutConfig() {
  return {
    type: "d3-force",
    link: {
      distance: 150,
      strength: 0.18
    },
    manyBody: {
      strength: -900
    },
    collide: {
      radius: 46,
      strength: 1,
      iterations: 4
    },
    center: {
      strength: 0.06
    },
    alphaDecay: 0.025,
    velocityDecay: 0.45,
    maxIteration: 1800
  };
}

/**
 * 获取 Collide 防重叠布局配置
 * 作为前置布局使用，确保节点无重叠
 */
export function getCollideLayoutConfig() {
  return {
    type: "collide",
    nodeSize: 80,
    padding: 10,       // 节点间距
    strength: 0.5,     // 碰撞强度
    iterations: 100    // 碰撞检测迭代次数
  };
}

/**
 * 获取 Dagre 层次布局配置（逆向导航模式）
 * 适用于展示学习路径的层级依赖关系
 */
export function getDagreLayoutConfig() {
  return {
    type: "dagre",
    rankdir: "LR",
    align: "UL",
    nodesep: 54,
    ranksep: 130,
    controlPoints: true,
    sortByCombo: false
  };
}

/**
 * 根据模式获取布局配置
 */
export function getLayoutConfig(layoutType: LayoutType) {
  return layoutType === "dagre" ? getDagreLayoutConfig() : getForceLayoutConfig();
}

/**
 * 获取节点配置（含锚点策略，避免边重叠）
 */
export function getNodeConfig() {
  return {
    type: "circle",
    style: {
      size: 70
    },
    // 四方向锚点，让边从节点边缘不同位置连接
    anchorPoints: [
      [0.5, 0],
      [1, 0.5],
      [0.5, 1],
      [0, 0.5]
    ]
  };
}

/**
 * 获取边配置（正交折线边，带严格路由避免穿过节点）
 */
export function getEdgeConfig() {
  return {
    type: "line",
    style: {
      lineWidth: 1,
      stroke: "#CBD5E1",
      endArrow: true,
      endArrowSize: 5
    }
  };
}
