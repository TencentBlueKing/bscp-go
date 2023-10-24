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
  - bscp-feed.site.bktencent.com:9511

log:
  alsoToStdErr: false
  logAppend: false
  logDir: ./log
  maxFileNum: 5
  maxPerFileSizeMB: 1024
  maxPerLineSizeKB: 2
  toStdErr: false
  verbosity: 5
