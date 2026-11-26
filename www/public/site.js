const demoMenu = document.querySelector('.demo-menu')
if (demoMenu) {
  const summary = demoMenu.querySelector('summary')
  document.addEventListener('click', (event) => {
    if (!demoMenu.contains(event.target)) demoMenu.open = false
  })
  demoMenu.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape' || !demoMenu.open) return
    demoMenu.open = false
    summary.focus()
  })
}
