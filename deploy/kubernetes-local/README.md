# Локальный запуск socialnet в Docker Desktop Kubernetes

Приложение перенесено в namespace `socialnet`. API, worker и backup работают
в одном Pod; PostgreSQL, Redis и Mailpit — в отдельных Deployment.
База, медиа, Redis и резервные копии используют отдельные PVC.

Адрес: http://localhost:8080. Скрипт `forward.ps1` поддерживает локальный
port-forward и переподключается после замены Pod. Задача Windows
`socialnet-kubernetes-forward` запускает его при входе пользователя.
Docker Desktop и Kubernetes должны быть включены.

```powershell
kubectl --context docker-desktop -n socialnet get pods,pvc
kubectl --context docker-desktop -n socialnet logs deployment/socialnet -c api
kubectl --context docker-desktop -n socialnet rollout restart deployment/socialnet
```

Образ `socialnet:photo-comments-20261120` импортирован в containerd локального
узла. Перед обновлением нужно собрать и импортировать новый образ,
изменить tag в Kustomization и применить `kubectl apply -k deploy/kubernetes-local`.
Secret `socialnet-secrets` создаётся отдельно: его нет в репозитории.

Исходные контейнеры Compose остановлены; тома сохранены. Согласованный
дамп БД и архивы медиа находятся вне репозитория в
`%LOCALAPPDATA%\socialnet\migration-*`. Там же хранится закрытая копия
конфигурации для восстановления. Не публикуйте её.

Для возврата после появления новых данных сначала сделайте дамп новой БД
и перенесите новые медиа обратно. Простой запуск старых контейнеров покажет
состояние на момент миграции. Нельзя одновременно запускать обе копии
для пользователей: записи в их базах начнут расходиться.

Это локальная конфигурация с HTTP на loopback, не публичное развёртывание.
HTTPS сайта требует отдельной настройки сертификата и точки входа.
Соединение мониторинга с Kubernetes API уже использует HTTPS и CA кластера.
Reset/Delete cluster может удалить локальные PVC; внешние копии обязательны
для восстановления после удаления кластера.
