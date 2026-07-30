import { describe, expect, it } from 'vitest'
import { parseRegisterSnippet } from './register'

describe('parseRegisterSnippet', () => {
  it('reads the snippet register-device prints', () => {
    expect(
      parseRegisterSnippet(`server_url: http://localhost:8080
device_id: win-desktop
token: 8f14e45fceea167a5a36dedd4bea2543
interval: 10s`),
    ).toEqual({
      server_url: 'http://localhost:8080',
      device_id: 'win-desktop',
      token: '8f14e45fceea167a5a36dedd4bea2543',
      interval: '10s',
    })
  })

  it('ignores the prose printed around the snippet', () => {
    expect(
      parseRegisterSnippet(`已注册设备 win-desktop。把下面几行粘进 config.yaml：

  server_url: https://cyberstalk.me
  device_id: win-desktop
  token: abc123

记得收紧 config.yaml 的权限。`),
    ).toEqual({
      server_url: 'https://cyberstalk.me',
      device_id: 'win-desktop',
      token: 'abc123',
    })
  })

  it('keeps a URL intact despite its slashes and colon', () => {
    expect(parseRegisterSnippet('server_url: http://192.168.1.10:8080')).toEqual({
      server_url: 'http://192.168.1.10:8080',
    })
  })

  it('unwraps quoted values', () => {
    expect(parseRegisterSnippet(`token: "0123456789"\ndevice_id: 'win-desktop'`)).toEqual({
      token: '0123456789',
      device_id: 'win-desktop',
    })
  })

  it('strips a trailing comment but keeps a "#" inside a token', () => {
    expect(parseRegisterSnippet('interval: 10s   # 每 10 秒上报一次')).toEqual({
      interval: '10s',
    })
    expect(parseRegisterSnippet('token: abc#123')).toEqual({ token: 'abc#123' })
  })

  it('skips commented-out and unknown keys', () => {
    expect(
      parseRegisterSnippet(`# token: 这行是注释
device_name: 我的台式机
device_id: win-desktop`),
    ).toEqual({ device_id: 'win-desktop' })
  })

  it('returns null when there is nothing to take', () => {
    for (const text of ['', '  ', '随便粘了点什么', 'device_id:', 'no colon here']) {
      expect(parseRegisterSnippet(text)).toBeNull()
    }
  })

  it('accepts keys in any case, as YAML readers would not but people do', () => {
    expect(parseRegisterSnippet('SERVER_URL: http://x:1')).toEqual({
      server_url: 'http://x:1',
    })
  })
})
