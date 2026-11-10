FROM python:3.12-slim

# Não gerar .pyc e não segurar logs em buffer (logs aparecem na hora no CloudWatch).
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY auditor ./auditor

# Usuário sem privilégios: se alguém explorar o container, não é root.
RUN useradd --create-home worker
USER worker

CMD ["python", "-m", "auditor.main"]
