export interface HostConfig {
    openrouterSecretArn?:string;
    hostname:string; region:string; dbHost:string; dbArn:string; ownerArn:string;
    image:string; caddyImage:string; logGroup:string; stack:string; behindLoadBalancer?:boolean;
}
// Values enter shell scripts only through JSON/base64, never shell interpolation.
export function bootstrap(config:HostConfig):string {
    const files:Record<string,string> = {
        'config.json': JSON.stringify(config),
        'Caddyfile': `${config.hostname} {\n header Strict-Transport-Security "max-age=31536000"\n reverse_proxy 127.0.0.1:8080\n}\n`,
        'refresh.py': `import json, subprocess, os, urllib.parse
from pathlib import Path
c=json.loads(Path('/opt/ajent/config.json').read_text())
def secret(arn):
    return json.loads(subprocess.check_output(['aws','secretsmanager','get-secret-value','--region',c['region'],'--secret-id',arn]))['SecretString']
pw=secret(c['dbArn'])
owner=secret(c['ownerArn'])
dsn='postgres://ajent:'+urllib.parse.quote(pw,safe='')+'@'+c['dbHost']+':5432/ajent?sslmode=verify-full&sslrootcert=/certs/rds.pem${config.behindLoadBalancer ? '&pool_max_conns=4' : ''}'
os.umask(0o077)
p=Path('/run/ajent.env.tmp')
p.write_text('AJENT_DSN='+dsn+'\\nAJENT_SETUP_TOKEN='+owner+'\\nAJENT_ORIGIN=https://'+c['hostname']+'\\nAJENT_TRUSTED_PROXIES=${config.behindLoadBalancer ? '10.87.1.0/24,10.87.2.0/24' : '127.0.0.1/32'}\\n')
if c.get('openrouterSecretArn'):
    key=secret(c['openrouterSecretArn'])
    if not key or any(ch in key for ch in ['\\n','\\r']):raise ValueError('Invalid chat secret format')
    with p.open('a') as f:f.write('OPENROUTER_API_KEY='+key+'\\n')
p.replace('/run/ajent.env')
registry=c['image'].split('/')[0]
pw=subprocess.check_output(['aws','ecr','get-login-password','--region',c['region']])
subprocess.run(['docker','login','--username','AWS','--password-stdin',registry],input=pw,check=True,stdout=subprocess.DEVNULL)
subprocess.run(['docker','pull',c['image']],check=True)
`,
        'run.py': `import json, os, sys
from pathlib import Path
c=json.loads(Path('/opt/ajent/config.json').read_text())
kind=sys.argv[1]
args=['docker','run','--rm','--name','ajent-'+kind,'--network','host','--log-driver','awslogs','--log-opt','awslogs-region='+c['region'],'--log-opt','awslogs-group='+c['logGroup'],'--log-opt','awslogs-stream='+kind,'--security-opt','no-new-privileges:true']
if kind=='app':
    args+=['--read-only','--cap-drop','ALL','--env-file','/run/ajent.env','-v','/opt/ajent/rds.pem:/certs/rds.pem:ro',c['image'],'-addr','${config.behindLoadBalancer ? '0.0.0.0:8080' : '127.0.0.1:8080'}']
else:
    args+=['-v','/opt/ajent/Caddyfile:/etc/caddy/Caddyfile:ro','-v','/var/lib/ajent/caddy-data:/data','-v','/var/lib/ajent/caddy-config:/config',c['caddyImage']]
os.execvp(args[0],args)
`,
        'metrics.py': `import json, subprocess, shutil
from pathlib import Path
c=json.loads(Path('/opt/ajent/config.json').read_text())
health=subprocess.run(['curl','--fail','--silent','--max-time','10','https://'+c['hostname']+'/healthz'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0
usage=shutil.disk_usage('/')
metrics=[{'MetricName':n,'Dimensions':[{'Name':'Stack','Value':c['stack']}],'Value':v} for n,v in [('Healthy',int(health)),('DiskUsedPercent',100*usage.used/usage.total)]]
subprocess.run(['aws','cloudwatch','put-metric-data','--region',c['region'],'--namespace','Ajent/Dogfood','--metric-data',json.dumps(metrics)],check=True)
`,
    };
    const units:Record<string,string> = {
        'ajent-app.service': `[Unit]
Description=Ajent application
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service
StartLimitIntervalSec=0
[Service]
ExecStartPre=/usr/bin/python3 /opt/ajent/refresh.py
ExecStartPre=-/usr/bin/docker rm -f ajent-app
ExecStart=/usr/bin/python3 /opt/ajent/run.py app
ExecStop=/usr/bin/docker stop -t 15 ajent-app
Restart=always
RestartSec=15
TimeoutStartSec=300
TimeoutStopSec=30
[Install]
WantedBy=multi-user.target
`,
        'ajent-proxy.service': `[Unit]
Description=Ajent TLS proxy
After=network-online.target docker.service
Requires=docker.service
StartLimitIntervalSec=0
[Service]
ExecStartPre=-/usr/bin/docker rm -f ajent-proxy
ExecStart=/usr/bin/python3 /opt/ajent/run.py proxy
ExecStop=/usr/bin/docker stop -t 15 ajent-proxy
Restart=always
RestartSec=15
TimeoutStartSec=300
TimeoutStopSec=30
[Install]
WantedBy=multi-user.target
`,
        'ajent-metrics.service': `[Unit]
Description=Ajent HTTPS and disk metrics
After=network-online.target
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /opt/ajent/metrics.py
TimeoutStartSec=50
`,
        'ajent-metrics.timer': `[Unit]
Description=Check Ajent each minute
[Timer]
OnBootSec=120
OnUnitActiveSec=60
[Install]
WantedBy=timers.target
`,
    };
    const write=(path:string,body:string)=>`printf '%s' '${Buffer.from(body).toString('base64')}' | base64 -d > ${path}`;
    return '#!/bin/bash\nset -euo pipefail\numask 077\n' +
        'dnf install -y docker awscli-2 python3\nsystemctl enable --now docker\n' +
        'install -d -m 755 /opt/ajent\ninstall -d -m 700 /var/lib/ajent/caddy-data /var/lib/ajent/caddy-config\n' +
        Object.entries(files).map(([name,body])=>write('/opt/ajent/'+name,body)).join('\n')+'\n'+
        'chmod 644 /opt/ajent/Caddyfile\ncurl --fail --retry 5 --proto =https --tlsv1.2 https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem -o /opt/ajent/rds.pem\nchmod 644 /opt/ajent/rds.pem\n'+
        Object.entries(units).map(([name,body])=>write('/etc/systemd/system/'+name,body)).join('\n')+'\n'+
        `systemctl daemon-reload\nsystemctl enable --now ajent-app ${config.behindLoadBalancer ? '' : 'ajent-proxy '}ajent-metrics.timer\n`;
}
