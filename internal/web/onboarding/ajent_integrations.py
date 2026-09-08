"""Install user-scoped integrations, preserving unrelated configuration."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

def atomic(path, value):
    path.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
    if path.is_symlink():raise ValueError('Symlinked config; configure this integration manually.')
    fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.ajent-')
    try:
        with os.fdopen(fd,'w') as f:
            json.dump(value,f,indent=2);f.write('\n');f.flush();os.fsync(f.fileno())
        os.replace(tmp,path)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)

def merge_server(path, section, config):
    if path.is_symlink():raise ValueError('Symlinked config; left unchanged.')
    before=path.read_text() if path.exists() else None
    current=json.loads(before) if before is not None else {}
    entries=current.setdefault(section,{})
    if 'ajent' in entries and entries['ajent']!=config:
        raise ValueError('An Ajent entry already exists with different settings; left unchanged.')
    if entries.get('ajent')==config:return
    entries['ajent']=config
    if before is not None:
        backup=path.with_name(path.name+'.ajent-backup-'+str(time.time_ns()))
        fd=os.open(backup,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        with os.fdopen(fd,'w') as f:f.write(before)
    atomic(path,current)

def run(args):
    # Vendor diagnostics can contain other MCP configuration. Never relay them.
    return subprocess.run(args,capture_output=True,text=True,timeout=30)

def install(directory, client, credential, *, skill_path=None):
    home=Path.home()
    env={'AJENT_CONFIG_DIR':str(directory),'AJENT_CREDENTIAL_FILE':str(credential)}
    def config(profile):
        return {'command':sys.executable,'args':[str(client),'mcp','--profile',profile,'--project-profile'],'env':env}
    results=[]
    for name in ['claude','codex']:
        if not shutil.which(name):continue
        try:
            existing=run([name,'mcp','get','ajent'])
            if existing.returncode==0:
                results.append(name+': existing Ajent MCP registration preserved; reload MCP to check the connection')
                continue
            elif name=='claude':
                result=run(['claude','mcp','add-json','--scope','user','ajent',json.dumps(config('claude-code'))])
                if result.returncode:raise ValueError('MCP registration needs attention.')
            else:
                result=run(['codex','mcp','add','--env','AJENT_CONFIG_DIR='+str(directory),'--env','AJENT_CREDENTIAL_FILE='+str(credential),'ajent','--',sys.executable,str(client),'mcp','--profile','codex','--project-profile'])
                if result.returncode:raise ValueError('MCP registration needs attention.')
            results.append(name+': MCP configured')
        except (OSError,ValueError,subprocess.TimeoutExpired):results.append(name+': setup incomplete; existing settings preserved, see '+str(directory/'mcp-example.json'))
    paths=[('Cursor',home/'.cursor'/'mcp.json','mcpServers','cursor',False),('Gemini CLI',home/'.gemini'/'settings.json','mcpServers','gemini-cli',False),('Copilot CLI',home/'.copilot'/'mcp-config.json','mcpServers','copilot',True)]
    vscode=home/'Library/Application Support/Code/User' if sys.platform=='darwin' else home/'.config/Code/User'
    paths.append(('VS Code',vscode/'mcp.json','servers','copilot-vscode',False))
    for name,path,section,profile,copilot in paths:
        if not path.parent.exists():continue
        try:
            value=config(profile)
            if copilot:value.update(type='local',tools=['*'])
            if section=='servers':value['type']='stdio'
            merge_server(path,section,value)
            results.append(name+': MCP configured')
        except (OSError,ValueError,TypeError,AttributeError):results.append(name+': setup incomplete; config left unchanged (comments or conflicting entry)')
    atomic(directory/'mcp-example.json',{'mcpServers':{'ajent':config('custom-agent')}})
    # Codex and compatible tools discover user skills here. Never overwrite an
    # independently installed skill with the same name.
    skill=skill_path if skill_path is not None else home/'.agents/skills/ajent-network/SKILL.md'
    source=directory/'marketplace/plugins/ajent/skills/network/SKILL.md'
    content=source.read_text()
    if not skill.exists() or skill.read_text()==content:
        skill.parent.mkdir(parents=True,exist_ok=True)
        if not skill.exists():
            fd=os.open(skill,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
            with os.fdopen(fd,'w') as f:f.write(content)
        results.append('Shared Ajent skill installed')
    else:results.append('Existing Ajent skill preserved')
    results.append('Plugin files updated locally; enable the plugin through your harness after reviewing its settings. MCP remains independently configured.')
    atomic(directory/'integration-status.json',{'results':results})
    for line in results:print(line)
    print('Reload MCP connections or start new agent sessions to pick up the integration. Existing sessions are not restarted.')
