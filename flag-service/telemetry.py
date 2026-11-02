"""Instrumentação OpenTelemetry compartilhada pelos serviços Python.

O destino vem das variáveis OTEL_EXPORTER_OTLP_* definidas no Deployment. Sem
elas, nada é ativado, o que mantém o desenvolvimento local simples. A função roda
em cada worker do gunicorn, já que o app não usa --preload.
"""
import logging
import os

from opentelemetry import metrics, trace
from opentelemetry.exporter.otlp.proto.http.metric_exporter import OTLPMetricExporter
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.flask import FlaskInstrumentor
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.instrumentation.psycopg2 import Psycopg2Instrumentor
from opentelemetry.instrumentation.requests import RequestsInstrumentor

log = logging.getLogger(__name__)


def init_telemetry(app, service_name):
    """Configura traces e métricas e instrumenta Flask e as dependências."""
    if not os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT"):
        log.info("OTEL_EXPORTER_OTLP_ENDPOINT não definida: telemetria desligada.")
        return

    resource = Resource.create({"service.name": os.getenv("OTEL_SERVICE_NAME", service_name)})

    tracer_provider = TracerProvider(resource=resource)
    tracer_provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    trace.set_tracer_provider(tracer_provider)

    reader = PeriodicExportingMetricReader(OTLPMetricExporter(), export_interval_millis=15000)
    metrics.set_meter_provider(MeterProvider(resource=resource, metric_readers=[reader]))

    FlaskInstrumentor().instrument_app(app, excluded_urls="health")
    RequestsInstrumentor().instrument()
    # Precisa acontecer antes de criar o pool de conexões.
    Psycopg2Instrumentor().instrument(skip_dep_check=True)
    log.info("Telemetria OpenTelemetry iniciada para %s.", service_name)
