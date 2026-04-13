import type { GraphEdge, GraphNode, GraphResponse } from "../api/graph";

export interface StyledGraphData {
  nodes: Array<Record<string, any>>;
  edges: Array<Record<string, any>>;
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

function styleNode(node: GraphNode, dimmed: boolean) {
  const preset = nodeStatusStyle[node.status ?? ""] ?? {
    fill: "#E2E8F0",
    stroke: "#64748B",
    lineWidth: 1.5
  };

  return {
    id: node.id,
    data: node,
    style: {
      labelText: node.label || node.id,
      fill: preset.fill,
      stroke: preset.stroke,
      lineWidth: preset.lineWidth,
      lineDash: preset.lineDash,
      radius: 14,
      opacity: dimmed ? 0.2 : 1,
      labelFill: "#0F172A",
      labelFontSize: 12,
      labelFontWeight: 500,
      halo: true,
      haloLineWidth: 8,
      haloStroke: "transparent"
    }
  };
}

function styleEdge(edge: GraphEdge, dimmed: boolean) {
  const isError = edge.status === "error";
  const isSupplement = edge.status === "supplement";

  return {
    id: edge.id || `${edge.source}-${edge.target}-${edge.label}`,
    source: edge.source,
    target: edge.target,
    data: edge,
    style: {
      labelText: edge.label,
      labelFill: "#475569",
      labelFontSize: 10,
      stroke: isError ? "#DC2626" : isSupplement ? "#7C3AED" : "#94A3B8",
      lineDash: isSupplement ? [8, 6] : undefined,
      lineWidth: isError ? 2.5 : 1.4,
      endArrow: true,
      opacity: dimmed ? 0.2 : 0.9
    }
  };
}

export function buildStyledGraph(
  graph: GraphResponse,
  focus?: { nodeIds: Set<string>; edgeIds: Set<string> }
): StyledGraphData {
  const hasFocus = Boolean(focus && (focus.nodeIds.size > 0 || focus.edgeIds.size > 0));

  const nodes = graph.nodes.map((node) =>
    styleNode(node, hasFocus ? !focus!.nodeIds.has(node.id) : false)
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
