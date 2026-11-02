#!/usr/bin/env bash
# Mostra as URLs e as senhas de Grafana e ArgoCD. Rode no CloudShell, fora da gravação.
set -euo pipefail
echo "Grafana: http://$(kubectl -n observability get svc grafana -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"
echo "  usuário: admin"
echo "  senha:   $(kubectl -n observability get secret grafana-admin -o jsonpath='{.data.admin-password}' | base64 -d)"
echo "ArgoCD:  https://$(kubectl -n argocd get svc argocd-server -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"
echo "  usuário: admin"
echo "  senha:   $(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"
