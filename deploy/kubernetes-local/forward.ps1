$ErrorActionPreference = 'Continue'
# Keep the local browser address stable when a Pod is replaced.
while ($true) {
    & kubectl --context docker-desktop -n socialnet port-forward --address 127.0.0.1 service/socialnet 8080:80
    Start-Sleep -Seconds 3
}
