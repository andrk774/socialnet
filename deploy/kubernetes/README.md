# socialnet в Kubernetes

Подготовленный стартовый вариант: один Pod с API, обработчиком медиа и
резервным копированием. Они используют общий PVC для медиа. Обновление
Recreate предусматривает короткий перерыв. Это не отказоустойчивый кластер;
не увеличивайте replicas с локальным медиа-хранилищем и backup-loop.
PostgreSQL 17, Redis и SMTP подключаются отдельно через Secret.
Нужны доступ к кластеру, default StorageClass и реестр образов.

## Подготовка

1. Соберите образ из корня: `docker build -t REGISTRY/socialnet:VERSION .`
   и загрузите его: `docker push REGISTRY/socialnet:VERSION`.
2. В `deployment.json` замените все три `socialnet:local` на этот образ.
   Для закрытого реестра добавьте imagePullSecrets.
3. Создайте namespace: `kubectl apply -f deploy/kubernetes/namespace.json`.
4. Создайте локальный файл секретов вне репозитория (не коммитьте его):

```dotenv
DATABASE_URL=postgres://USER:PASSWORD@DB:5432/socialnet?sslmode=require
REDIS_URL=rediss://:PASSWORD@REDIS:6379/0
JWT_SECRET=RANDOM_SECRET_AT_LEAST_32_BYTES
BACKUP_ENC_KEY=RANDOM_64_HEX_CHARACTERS
TOTP_ENC_KEY=RANDOM_64_HEX_CHARACTERS
APP_URL=https://YOUR_DOMAIN
SMTP_HOST=YOUR_SMTP
SMTP_PORT=587
SMTP_USER=YOUR_USER
SMTP_PASSWORD=YOUR_PASSWORD
MAIL_FROM=socialnet <no-reply@YOUR_DOMAIN>
```

Используйте действительные значения. Для переноса существующих аккаунтов
сохраните исходные JWT_SECRET и TOTP_ENC_KEY, перенесите БД и медиа.
Пользователю БД нужны права миграций; проверка резервных копий требует
создания временной базы. Настройте внешнее хранилище копий через BACKUP_S3_*
по основной документации deploy; PVC в том же кластере не защищает от его потери.

```sh
kubectl -n socialnet create secret generic socialnet-secrets --from-env-file=/secure/socialnet.env
kubectl kustomize deploy/kubernetes
kubectl apply -k deploy/kubernetes
kubectl -n socialnet rollout status deployment/socialnet
```

Публикация: подключите Service socialnet:80 к вашему Ingress/Gateway с HTTPS.
Тип контроллера и домен здесь намеренно не заданы. COOKIE_SECURE включён.
Для проверки через `kubectl -n socialnet port-forward svc/socialnet 8080:80`
по HTTP временно установите COOKIE_SECURE=false и APP_URL=http://localhost:8080
в тестовом окружении. Не переносите эту настройку в публичный запуск.

## Мониторинг

В админке → Система → Kubernetes показываются поды, готовность контейнеров,
перезапуски, имена узлов, развёртывания и события выбранного namespace.
Это не метрики CPU/RAM узлов; для них нужен отдельный metrics-server/Prometheus.
Role разрешает только list pods/events/deployments, без Secrets, exec, логов
и управления ресурсами. Токен смонтирован только в API и перечитывается
при каждом опросе для поддержки ротации. TLS проверяется по CA кластера.

Для существующего кластера из внешнего API socialnet задайте KUBE_API_URL
(HTTPS без пути), KUBE_NAMESPACE, KUBE_TOKEN_FILE и KUBE_CA_FILE.
Смонтируйте файлы только для чтения; обеспечьте обновление токена.
Для Docker Compose используйте отдельный override с environment и volumes.
Не загружайте kubeconfig администратора и не передавайте токены в браузер.

Проверка прав:
`kubectl auth can-i list pods --as=system:serviceaccount:socialnet:socialnet-monitor -n socialnet`.
Без настроенного endpoint интерфейс честно показывает «Кластер не подключён».

Основа: [доступ к API из Pod](https://kubernetes.io/docs/tasks/run-application/access-api-from-pod/),
[Kustomize](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization/).
