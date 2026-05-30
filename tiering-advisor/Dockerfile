# ── Build stage ───────────────────────────────────────────────────────────────
# Install dependencies in a separate layer so they are cached independently
# from source changes.
FROM python:3.11-slim AS builder

WORKDIR /install

COPY requirements.txt .

# --no-cache-dir   : skip the on-disk pip cache → smaller layer
# --prefix /install: install into a relocatable directory for the final copy
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

# ── Final stage ───────────────────────────────────────────────────────────────
# python:3.11-slim base ≈ 45 MB; scikit-learn + numpy + pandas ≈ 130 MB
# Total image target: < 200 MB
FROM python:3.11-slim

# Non-root user for least-privilege execution
RUN adduser --disabled-password --no-create-home appuser

WORKDIR /app

# Copy installed packages from the builder layer
COPY --from=builder /install /usr/local

# Copy application source
COPY main.py .

EXPOSE 8081

USER appuser

# Uvicorn with a single worker; scale horizontally via replicas instead.
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8081", "--workers", "1"]
