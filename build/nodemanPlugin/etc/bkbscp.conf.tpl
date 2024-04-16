# 插件全局配置
path.logs: /var/log/gse
path.data: /var/lib/gse
path.pid: /var/run/gse

# 业务id
biz: {{ 业务ID }}
apps: {% for app in 服务 %}
  - name: {{ app.服务名称 }}
    labels: {% for label in app.标签 %}
        {{ label.key }}: {{ label.value }}{% endfor %}
    {% endfor %}
# token
token: {{ 服务密钥 }}
temp_dir: {{ 临时目录 }}
{% if 全局标签 %}labels:
{% for label in 全局标签 %}{{ "-" if loop.first else " "  }} {{ label.key }}: "{{ label.value }}"
{% endfor %}
{% endif %}

feed_addrs:
  - {{ Feed服务地址 }}
