# Локальный запуск socialnet в Docker Desktop Kubernetes

Приложение перенесено в namespace `socialnet`. API, worker и backup работают
в одном Pod; PostgreSQL, Redis и Mailpit — в отдельных Deployment.
База, медиа, Redis и резервные копии используют отдельные PVC.

Адрес: http://localhost:8080. Скрипт `forward.ps1` поддерживает локальный
port-forward и переподключается после замены Pod. Задача Windows
`socialnet-kubernetes-forward` запускает его при входе пользователя.
Docker Desktop и Kubernetes должны быть включены.

Проверка подключения без запуска постоянного процесса:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File deploy/kubernetes-local/forward.ps1 -CheckOnly
Start-ScheduledTask -TaskName socialnet-kubernetes-forward
```

Чтобы существующая задача запускала подключение без окна Windows Terminal,
обновите её действие из корня проекта:

```powershell
$launcher = (Resolve-Path deploy/kubernetes-local/forward-hidden.vbs).Path
$action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\wscript.exe" -Argument ('//B //NoLogo "{0}"' -f $launcher)
Set-ScheduledTask -TaskName socialnet-kubernetes-forward -Action $action
```

`forward-hidden.vbs` запускает PowerShell скрыто и ждёт его завершения,
сохраняя контроль задачи Windows. Сам `kubectl` запускается с
`CreateNoWindow`, поэтому переподключение также не создаёт окно консоли.

`forward.ps1` ждёт готовности API кластера и восстанавливает подключение после
замены Pod с интервалом 15 секунд. Журналы находятся в
`%LOCALAPPDATA%\socialnet\port-forward`. Для задачи Windows рекомендуется
отключить ограничение времени выполнения, включить запуск при пропущенном
старте и три перезапуска при сбое с интервалом одна минута.

После обновления от 21.11.2026 API, worker и backup используют Go 1.27.4.
Правила доступа к фотографиям и требования к внешнему S3/CDN описаны в
[MEDIA_SECURITY.md](../MEDIA_SECURITY.md).

```powershell
kubectl --context docker-desktop -n socialnet get pods,pvc
kubectl --context docker-desktop -n socialnet logs deployment/socialnet -c api
kubectl --context docker-desktop -n socialnet rollout restart deployment/socialnet
```

Образ `socialnet:chat-20261122` импортирован в containerd локального
узла. Перед обновлением нужно собрать и импортировать новый образ,
изменить tag в Kustomization и применить `kubectl apply -k deploy/kubernetes-local`.
Secret `socialnet-secrets` создаётся отдельно: его нет в репозитории.

22.11.2026 обновлены API, worker и backup; применены миграции 0043–0045
для людей рядом, редактирования сообщений и приватных видеосообщений.
После обновления проверены готовность всех трёх контейнеров, `/health`
и выдача нового клиентского модуля `chatmedia.js` через localhost:8080.
Дамп БД перед обновлением сохранён в
`%LOCALAPPDATA%\socialnet\backup-chat-20261122\database.dump`.

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
