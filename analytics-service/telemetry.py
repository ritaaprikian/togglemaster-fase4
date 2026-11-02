"""Instrumentação OpenTelemetry compartilhada pelos serviços Python.

O destino vem das variáveis OTEL_EXPORTER_OTLP_* definidas no Deployment. Sem
elas, nada é ativado, o que mantém o desenvolvimento local simples. A função roda
em cada worker do gunicorn, já que o app não usa --preload.
"""
import logging
import os
import socket

from opentelemetry import metrics, trace
from opentelemetry.exporter.otlp.proto.http.metric_exporter import OTLPMetricExporter
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.flask import FlaskInstrumentor
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.sdk.metrics.view import ExplicitBucketHistogramAggregation, View
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.instrumentation.botocore import BotocoreInstrumentor

log = logging.getLogger(__name__)

# Faixas em segundos. As faixas padrão do SDK são para milissegundos: com elas, toda
# requisição cairia na primeira faixa e a latência p95 apareceria sempre perto de 5 s.
HTTP_DURATION_BUCKETS = (0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10)


def init_telemetry(app, service_name):
    """Configura traces e métricas e instrumenta Flask e as dependências."""
    if not os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT"):
        log.info("OTEL_EXPORTER_OTLP_ENDPOINT não definida: telemetria desligada.")
        return

    # Cada worker do gunicorn precisa de identidade própria: sem isso, os contadores
    # dos workers caem na mesma série no Prometheus e as taxas ficam incorretas.
    resource = Resource.create({
        "service.name": os.getenv("OTEL_SERVICE_NAME", service_name),
        "service.instance.id": f"{socket.gethostname()}-{os.getpid()}",
    })

    tracer_provider = TracerProvider(resource=resource)
    tracer_provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    trace.set_tracer_provider(tracer_provider)

    reader = PeriodicExportingMetricReader(OTLPMetricExporter(), export_interval_millis=15000)
    duration_view = View(
        instrument_name="http.server.request.duration",
        aggregation=ExplicitBucketHistogramAggregation(HTTP_DURATION_BUCKETS),
    )
    metrics.set_meter_provider(MeterProvider(resource=resource, metric_readers=[reader], views=[duration_view]))

    FlaskInstrumentor().instrument_app(app, excluded_urls="health")
    BotocoreInstrumentor().instrument()
    log.info("Telemetria OpenTelemetry iniciada para %s.", service_name)
