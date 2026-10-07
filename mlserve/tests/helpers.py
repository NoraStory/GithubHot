"""测试工具：构造请求载荷。"""
FEATURE_DIM = 16


def node(i: int, bias: float = 0.0):
    return {"id": f"fp_{i}", "features": [bias + (j % 7) * 0.1 for j in range(FEATURE_DIM)]}


def payload(n: int = 3, bias: float = 0.0, edges=None):
    if edges is None:
        if n >= 3:
            edges = [[0, 1], [1, 2]]
        elif n == 2:
            edges = [[0, 1]]
        else:
            edges = []
    return {"nodes": [node(i, bias) for i in range(n)], "edges": edges}
