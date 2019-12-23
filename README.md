# Git Viewer

View specific directories of private git repositories.

Example `config.yml` file (to be placed in `repos` directory in working directory):

```yaml
auth:
  github.com:
    username: mrbbot
    password: <access_token>
repos:
  all:
    url: https://github.com/mrbbot/gitviewer
    dir: .
  just-static:
    url: mrbbot/gitviewer
    dir: static
  cmd:
    url: gitviewer
    dir: cmd
```