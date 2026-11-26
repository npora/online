const playground = "https://request.npora.live/#playground";

function followPlaygroundLink() {
  if (location.pathname === "/" && location.hash === "#playground") {
    location.replace(playground);
  }
}

followPlaygroundLink();
window.addEventListener("hashchange", followPlaygroundLink);
