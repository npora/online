const tabs = [...document.querySelectorAll('[data-language]')]
const panels = {
  node: document.querySelector('#panel-node'),
  go: document.querySelector('#panel-go')
}
const frontendEnv = document.querySelector('#env-command')

function selectLanguage(language) {
  for (const item of tabs) item.setAttribute('aria-selected', String(item.dataset.language === language))
  for (const [name, panel] of Object.entries(panels)) panel.hidden = name !== language
  if (frontendEnv) frontendEnv.textContent = `VITE_API_URL=http://localhost:${language === 'go' ? '8080' : '3000'}`
}

for (const tab of tabs) {
  tab.addEventListener('click', () => selectLanguage(tab.dataset.language))
}

selectLanguage('node')

for (const button of document.querySelectorAll('[data-copy]')) {
  button.addEventListener('click', async () => {
    const code = document.getElementById(button.dataset.copy)?.innerText ?? ''
    try {
      await navigator.clipboard.writeText(code)
      button.textContent = '已复制'
    } catch {
      button.textContent = '复制失败'
    }
    window.setTimeout(() => { button.textContent = '复制' }, 1400)
  })
}
