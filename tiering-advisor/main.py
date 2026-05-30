"""
CloudBridge Tiering Advisor
===========================
A FastAPI microservice that connects to the CloudBridge PostgreSQL database,
analyses file-access patterns using rule-based logic and sklearn IsolationForest,
and recommends (or applies) storage-tier changes (hot / warm / cold).

Design notes
------------
* The advisor has *read-only* DB access for analysis.
* `POST /apply` updates the `files.tier` column directly and inserts
  `tier_move` sync_jobs so the CloudBridge worker pool picks them up.
* IsolationForest detects files with anomalous access-count spikes that
  the simple time-threshold rules would miss — these are promoted to hot.
"""

import logging
import os
from contextlib import asynccontextmanager
from datetime import datetime, timezone
from typing import Any, Optional
from uuid import UUID, uuid4

import asyncpg
import httpx
import numpy as np
import pandas as pd
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sklearn.ensemble import IsolationForest

# ── Logging ───────────────────────────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)-8s %(name)s: %(message)s",
)
logger = logging.getLogger("tiering-advisor")

# ── Config ────────────────────────────────────────────────────────────────────
DATABASE_URL: str = os.getenv(
    "DATABASE_URL",
    "postgresql://postgres:postgres@localhost:5432/cloudbridge",
)
CLOUDBRIDGE_URL: str = os.getenv("CLOUDBRIDGE_URL", "http://localhost:8080")

# ── Pydantic schemas ──────────────────────────────────────────────────────────


class TieringRecommendation(BaseModel):
    file_path: str
    current_tier: str
    recommended_tier: str
    reason: str
    confidence_score: float


class AnalysisResponse(BaseModel):
    namespace_id: str
    analyzed_at: datetime
    total_files: int
    recommendations: list[TieringRecommendation]
    anomalies_detected: int


class ApplyResponse(BaseModel):
    namespace_id: str
    applied_at: datetime
    tiers_updated: int
    jobs_created: int
    errors: list[str]


class TierDistribution(BaseModel):
    hot: int = 0
    warm: int = 0
    cold: int = 0


class TieringReport(BaseModel):
    namespace_id: str
    report_at: datetime
    total_files: int
    total_size_bytes: int
    tier_distribution: TierDistribution
    recommendations: list[TieringRecommendation]
    anomalies_detected: int
    potential_savings_note: str


# ── TieringAnalyzer ───────────────────────────────────────────────────────────


class TieringAnalyzer:
    """
    Analyses file-access patterns from PostgreSQL and produces tier recommendations.

    Tiering rules
    -------------
    HOT  → accessed within last 7 days  OR access_count > 100
    WARM → accessed within last 30 days OR access_count > 10
    COLD → not accessed in 30+ days     AND access_count ≤ 10

    Anomaly detection
    -----------------
    IsolationForest is trained on (access_count, days_since_access, size_bytes).
    Files flagged as anomalous with a non-trivial access count are promoted to
    HOT regardless of their time threshold, catching sudden access spikes.
    """

    # Rule thresholds
    HOT_DAYS: int = 7
    WARM_DAYS: int = 30
    HOT_COUNT: int = 100
    WARM_COUNT: int = 10

    # IsolationForest: expect ~5 % of files to be genuine anomalies
    CONTAMINATION: float = 0.05

    _QUERY = """
        SELECT  id,
                path,
                access_count,
                last_accessed_at,
                size_bytes,
                tier,
                cloud_synced
        FROM    files
        WHERE   namespace_id = $1
    """

    def __init__(self, pool: asyncpg.Pool) -> None:
        self._pool = pool

    # ── DB fetch ──────────────────────────────────────────────────────────────

    async def fetch_files(self, namespace_id: str) -> pd.DataFrame:
        """Return a DataFrame with one row per file in the namespace."""
        rows = await self._pool.fetch(self._QUERY, UUID(namespace_id))
        if not rows:
            return pd.DataFrame()

        now = datetime.now(timezone.utc)
        records: list[dict[str, Any]] = []
        for r in rows:
            last_at: datetime = r["last_accessed_at"]
            if last_at.tzinfo is None:
                last_at = last_at.replace(tzinfo=timezone.utc)
            records.append(
                {
                    "id": str(r["id"]),
                    "path": r["path"],
                    "access_count": int(r["access_count"]),
                    "last_accessed_at": last_at,
                    "days_since_access": (now - last_at).days,
                    "size_bytes": int(r["size_bytes"]),
                    "tier": r["tier"],
                    "cloud_synced": bool(r["cloud_synced"]),
                }
            )
        return pd.DataFrame(records)

    # ── Rule-based classification ─────────────────────────────────────────────

    def _classify(self, row: pd.Series) -> tuple[str, str, float]:
        """Return *(recommended_tier, reason, confidence)*."""
        days: int = int(row["days_since_access"])
        count: int = int(row["access_count"])

        if days <= self.HOT_DAYS or count > self.HOT_COUNT:
            if days <= self.HOT_DAYS and count > self.HOT_COUNT:
                return (
                    "hot",
                    f"Recently accessed ({days}d ago) and high access count ({count})",
                    0.99,
                )
            if days <= self.HOT_DAYS:
                return (
                    "hot",
                    f"Accessed {days}d ago — within {self.HOT_DAYS}d hot threshold",
                    0.95,
                )
            return (
                "hot",
                f"High access count ({count} > {self.HOT_COUNT})",
                0.90,
            )

        if days <= self.WARM_DAYS or count > self.WARM_COUNT:
            if days <= self.WARM_DAYS:
                return (
                    "warm",
                    f"Accessed {days}d ago — within {self.WARM_DAYS}d warm threshold",
                    0.85,
                )
            return (
                "warm",
                f"Moderate access count ({count} > {self.WARM_COUNT})",
                0.80,
            )

        return (
            "cold",
            f"Inactive for {days}d with only {count} total accesses",
            0.90,
        )

    # ── Anomaly detection ─────────────────────────────────────────────────────

    def _detect_anomalies(self, df: pd.DataFrame) -> pd.Series:
        """
        Apply IsolationForest to (access_count, days_since_access, size_bytes).
        Returns a boolean Series (True = anomaly) aligned to df's index.
        Requires ≥ 3 rows; returns all-False otherwise.
        """
        if len(df) < 3:
            return pd.Series([False] * len(df), index=df.index)

        features = (
            df[["access_count", "days_since_access", "size_bytes"]]
            .fillna(0)
            .astype(np.float64)
        )
        iso = IsolationForest(
            n_estimators=100,
            contamination=self.CONTAMINATION,
            random_state=42,
        )
        # fit_predict: -1 = anomaly, +1 = normal
        preds = iso.fit_predict(features)
        return pd.Series(preds == -1, index=df.index)

    # ── Core analysis ─────────────────────────────────────────────────────────

    def analyze(
        self, df: pd.DataFrame
    ) -> tuple[list[TieringRecommendation], int]:
        """
        Return *(recommendations, anomaly_count)*.
        Only files whose recommended tier differs from the current tier are included.
        """
        if df.empty:
            return [], 0

        anomaly_mask = self._detect_anomalies(df)
        anomaly_count = int(anomaly_mask.sum())
        recommendations: list[TieringRecommendation] = []

        for idx, row in df.iterrows():
            recommended, reason, confidence = self._classify(row)
            is_anomaly: bool = bool(anomaly_mask[idx])

            # Anomaly boost: unexpected access spike → promote to hot
            if is_anomaly and int(row["access_count"]) > self.WARM_COUNT:
                if recommended != "hot":
                    reason = f"Anomalous access spike detected — {reason}"
                    recommended = "hot"
                    confidence = min(confidence + 0.05, 1.0)

            if recommended != row["tier"]:
                recommendations.append(
                    TieringRecommendation(
                        file_path=row["path"],
                        current_tier=row["tier"],
                        recommended_tier=recommended,
                        reason=reason,
                        confidence_score=round(confidence, 3),
                    )
                )

        # Sort: highest-confidence changes first
        recommendations.sort(key=lambda r: r.confidence_score, reverse=True)
        return recommendations, anomaly_count


# ── App lifecycle ─────────────────────────────────────────────────────────────

_pool: Optional[asyncpg.Pool] = None


@asynccontextmanager
async def lifespan(app: FastAPI):  # type: ignore[misc]
    global _pool
    logger.info("Opening PostgreSQL connection pool …")
    _pool = await asyncpg.create_pool(
        DATABASE_URL,
        min_size=2,
        max_size=10,
        command_timeout=30,
    )
    logger.info("PostgreSQL pool ready (DATABASE_URL=%s)", _redact(DATABASE_URL))
    yield
    if _pool:
        await _pool.close()
        logger.info("PostgreSQL pool closed")


def _redact(url: str) -> str:
    """Remove the password from a connection string for safe logging."""
    for i, ch in enumerate(url):
        if ch == "@":
            return "postgresql://***@" + url[i + 1 :]
    return url


app = FastAPI(
    title="CloudBridge Tiering Advisor",
    description=(
        "Analyses file-access patterns in the CloudBridge namespace and "
        "recommends or applies hot/warm/cold storage-tier changes."
    ),
    version="1.0.0",
    lifespan=lifespan,
)


def _get_pool() -> asyncpg.Pool:
    if _pool is None:
        raise HTTPException(status_code=503, detail="Database pool not ready")
    return _pool


# ── Endpoints ─────────────────────────────────────────────────────────────────


@app.get("/health", tags=["observability"])
async def health() -> dict:
    """Liveness + readiness probe. Checks DB connectivity."""
    pool = _get_pool()
    try:
        await pool.fetchval("SELECT 1")
        db = "connected"
    except Exception as exc:
        logger.warning("DB health check failed: %s", exc)
        db = f"error: {exc}"
    return {"status": "ok", "db": db, "version": "1.0.0"}


@app.post(
    "/api/v1/analyze/{namespace_id}",
    response_model=AnalysisResponse,
    tags=["tiering"],
    summary="Analyse access patterns and return tier recommendations (read-only).",
)
async def analyze(namespace_id: str) -> AnalysisResponse:
    pool = _get_pool()
    analyzer = TieringAnalyzer(pool)

    try:
        df = await analyzer.fetch_files(namespace_id)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=f"Invalid namespace_id: {exc}")
    except Exception as exc:
        logger.error("fetch_files failed for %s: %s", namespace_id, exc)
        raise HTTPException(status_code=500, detail=str(exc))

    recs, anomaly_count = analyzer.analyze(df)
    return AnalysisResponse(
        namespace_id=namespace_id,
        analyzed_at=datetime.now(timezone.utc),
        total_files=len(df),
        recommendations=recs,
        anomalies_detected=anomaly_count,
    )


@app.post(
    "/api/v1/apply/{namespace_id}",
    response_model=ApplyResponse,
    tags=["tiering"],
    summary="Apply tier recommendations: update DB tiers and enqueue tier_move jobs.",
)
async def apply_recommendations(namespace_id: str) -> ApplyResponse:
    """
    1. Runs the same analysis as /analyze.
    2. Updates `files.tier` in the CloudBridge database.
    3. Inserts `tier_move` sync_jobs so the CloudBridge worker pool handles
       the cloud-side storage-class changes.
    4. Verifies CloudBridge gateway reachability via GET /health.
    """
    pool = _get_pool()
    analyzer = TieringAnalyzer(pool)
    errors: list[str] = []

    try:
        df = await analyzer.fetch_files(namespace_id)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=f"Invalid namespace_id: {exc}")
    except Exception as exc:
        raise HTTPException(status_code=500, detail=str(exc))

    recs, _ = analyzer.analyze(df)
    if not recs:
        return ApplyResponse(
            namespace_id=namespace_id,
            applied_at=datetime.now(timezone.utc),
            tiers_updated=0,
            jobs_created=0,
            errors=[],
        )

    # Build a path → row lookup for the file IDs and cloud_synced flag
    file_index: dict[str, pd.Series] = {
        row["path"]: row for _, row in df.iterrows()
    }

    tiers_updated = 0
    jobs_created = 0

    for rec in recs:
        file_row = file_index.get(rec.file_path)
        if file_row is None:
            errors.append(f"File not found in snapshot: {rec.file_path}")
            continue

        # ── 1. Update tier in DB ──────────────────────────────────────────────
        try:
            result = await pool.execute(
                """
                UPDATE files
                SET    tier = $1
                WHERE  namespace_id = $2
                  AND  path        = $3
                """,
                rec.recommended_tier,
                UUID(namespace_id),
                rec.file_path,
            )
            if result == "UPDATE 1":
                tiers_updated += 1
                logger.info(
                    "Tier updated: %s  %s → %s",
                    rec.file_path,
                    rec.current_tier,
                    rec.recommended_tier,
                )
        except Exception as exc:
            msg = f"DB update failed for {rec.file_path}: {exc}"
            logger.error(msg)
            errors.append(msg)
            continue

        # ── 2. Insert tier_move sync_job for cloud-synced files ───────────────
        if file_row.get("cloud_synced", False):
            try:
                await pool.execute(
                    """
                    INSERT INTO sync_jobs (
                        id, namespace_id, file_id, operation,
                        status, retry_count, error_message,
                        bytes_transferred, created_at
                    ) VALUES (
                        $1, $2, $3, 'tier_move',
                        'pending', 0, '',
                        0, NOW()
                    )
                    """,
                    uuid4(),
                    UUID(namespace_id),
                    UUID(file_row["id"]),
                )
                jobs_created += 1
            except Exception as exc:
                msg = f"sync_job insert failed for {rec.file_path}: {exc}"
                logger.warning(msg)
                errors.append(msg)

    # ── 3. Verify CloudBridge gateway is reachable ────────────────────────────
    try:
        async with httpx.AsyncClient(timeout=5) as client:
            resp = await client.get(f"{CLOUDBRIDGE_URL}/health")
            resp.raise_for_status()
            logger.info(
                "CloudBridge reachable, workers will process %d tier_move jobs",
                jobs_created,
            )
    except Exception as exc:
        msg = f"CloudBridge health check failed ({CLOUDBRIDGE_URL}): {exc}"
        logger.warning(msg)
        errors.append(msg)

    return ApplyResponse(
        namespace_id=namespace_id,
        applied_at=datetime.now(timezone.utc),
        tiers_updated=tiers_updated,
        jobs_created=jobs_created,
        errors=errors,
    )


@app.get(
    "/api/v1/report/{namespace_id}",
    response_model=TieringReport,
    tags=["tiering"],
    summary="Full tiering report: distribution, recommendations, anomalies, savings estimate.",
)
async def report(namespace_id: str) -> TieringReport:
    pool = _get_pool()
    analyzer = TieringAnalyzer(pool)

    try:
        df = await analyzer.fetch_files(namespace_id)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=f"Invalid namespace_id: {exc}")
    except Exception as exc:
        raise HTTPException(status_code=500, detail=str(exc))

    recs, anomaly_count = analyzer.analyze(df)

    dist = TierDistribution()
    total_size = 0

    if not df.empty:
        counts = df["tier"].value_counts().to_dict()
        dist.hot = counts.get("hot", 0)
        dist.warm = counts.get("warm", 0)
        dist.cold = counts.get("cold", 0)
        total_size = int(df["size_bytes"].sum())

    # Estimate bytes eligible for cheaper storage (hot → warm/cold demotion)
    moveable = 0
    if not df.empty:
        file_sizes = df.set_index("path")["size_bytes"].to_dict()
        for r in recs:
            if r.current_tier == "hot" and r.recommended_tier in ("warm", "cold"):
                moveable += file_sizes.get(r.file_path, 0)

    savings_gb = moveable / (1024 ** 3)
    savings_note = (
        f"{savings_gb:.2f} GB eligible for cheaper tiers "
        f"(warm ≈ 50 % cheaper, cold ≈ 80 % cheaper than hot storage)"
    )

    return TieringReport(
        namespace_id=namespace_id,
        report_at=datetime.now(timezone.utc),
        total_files=len(df),
        total_size_bytes=total_size,
        tier_distribution=dist,
        recommendations=recs,
        anomalies_detected=anomaly_count,
        potential_savings_note=savings_note,
    )
