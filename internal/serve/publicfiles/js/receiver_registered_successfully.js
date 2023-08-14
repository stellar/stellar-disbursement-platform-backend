// Purpose: to let other windows know that the receiver has been registered successfully
postMessage("verified");

document.addEventListener("DOMContentLoaded", function() {
  const button = document.getElementById('backToHomeButton');

  button.addEventListener('click', function(event) {
    backToHome(event);
  });
});


function backToHome(event) {
  window.close();

  // Purpose: to let other windows know that this window has been closed
  postMessage('close');
}