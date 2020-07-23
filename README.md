# ipmi_pushgateway
start pushgateway container  
```
docker run -d
  --name=pushgateway \
  -p 9091:9091 \
  prom/pushgateway
```  
vim prometheus.yml  
```
global:
  scrape_interval:     60s
  evaluation_interval: 60s
 
scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ['localhost:9090']
        labels:
          instance: prometheus
 
  - job_name: linux
    static_configs:
      - targets: ['192.168.91.132:9100']
        labels:
          instance: localhost
  - job_name: pushgateway
    static_configs:
      - targets: ['192.168.91.132:9091']
        labels:
          instance: pushgateway
```  
start prometheus container
```
docker run  -d \
  -p 9090:9090 \
  -v /opt/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml  \
  prom/prometheus
```

