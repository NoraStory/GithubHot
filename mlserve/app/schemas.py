"""请求/响应模型与载荷校验（子图 ≤ MAX_NODES、特征维度一致、边索引合法）。"""
from typing import List, Tuple

from pydantic import BaseModel, Field, field_validator

from app import config


class Node(BaseModel):
    id: str = Field(min_length=1, max_length=128)
    features: List[float]

    @field_validator("features")
    @classmethod
    def finite(cls, v: List[float]) -> List[float]:
        if len(v) != config.FEATURE_DIM:
            raise ValueError(f"features 维度必须为 {config.FEATURE_DIM}（当前 {len(v)}）")
        if any(not (-1e12 < x < 1e12) for x in v):
            raise ValueError("features 含非法数值")
        return v


class ScoreRequest(BaseModel):
    nodes: List[Node]
    edges: List[Tuple[int, int]] = Field(default_factory=list)

    @field_validator("nodes")
    @classmethod
    def node_budget(cls, v: List[Node]) -> List[Node]:
        if not (1 <= len(v) <= config.MAX_NODES):
            raise ValueError(f"节点数必须在 1..{config.MAX_NODES}（当前 {len(v)}）")
        ids = [n.id for n in v]
        if len(set(ids)) != len(ids):
            raise ValueError("节点 id 重复")
        return v

    @field_validator("edges")
    @classmethod
    def edge_range(cls, v: List[Tuple[int, int]], info) -> List[Tuple[int, int]]:
        n = len(info.data.get("nodes") or [])
        for a, b in v:
            if not (0 <= a < n and 0 <= b < n):
                raise ValueError(f"边索引越界: ({a},{b})，节点数 {n}")
        if len(v) > config.MAX_NODES * config.MAX_NODES:
            raise ValueError("边数超限")
        return v


class ScoreResponse(BaseModel):
    scores: dict
    embeddings: dict
    model_version: str
    elapsed_ms: int
