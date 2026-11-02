#!/usr/bin/env bash
# "A Prova Real": injeta 500 no /validate de todos os Pods do auth-service.
# A falha fica só na memória dos Pods, então apenas o rollout restart a corrige.
set -euo pipefail
NS=togglemaster

for pod in $(kubectl -n "$NS" get pods -l app=auth-service -o jsonpath='{.items[*].metadata.name}'); do
  echo "Injetando falha em $pod"
  kubectl -n "$NS" exec "$pod" -- sh -c \
    'wget -q -O /dev/null --post-data="" --header="Authorization: Bearer $MASTER_KEY" http://localhost:8001/admin/chaos'
done

echo "Falha ativa. Acompanhe os Pods reiniciando:"
kubectl -n "$NS" get pods -l app=auth-service -w
