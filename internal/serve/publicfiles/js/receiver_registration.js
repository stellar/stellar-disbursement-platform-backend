const WalletRegistration = {
  jwtToken: "",
};

function getJwtToken() {
  const tokenEl = document.querySelector("[data-jwt-token]");

  if (tokenEl) {
    return tokenEl.innerHTML;
  }
}

function toggleNotification(type, { parentEl, title, message, isVisible }) {
  const titleEl = parentEl.querySelector(`[data-section-${type}-title]`);
  const messageEl = parentEl.querySelector(`[data-section-${type}-message`);

  if (titleEl && messageEl) {
    if (isVisible) {
      parentEl.style.display = "flex";
      titleEl.innerHTML = title;
      messageEl.innerHTML = message;
    } else {
      parentEl.style.display = "none";
      titleEl.innerHTML = "";
      messageEl.innerHTML = "";
    }
  }
}

function toggleErrorNotification(parentEl, title, message, isVisible) {
  toggleNotification("error", { parentEl, title, message, isVisible });
}

function toggleSuccessNotification(parentEl, title, message, isVisible) {
  toggleNotification("success", { parentEl, title, message, isVisible });
}

async function sendSms(phoneNumber, reCAPTCHAToken, onSuccess, onError) {
  if (phoneNumber && reCAPTCHAToken) {
    try {
      const request = await fetch("/wallet-registration/otp", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${WalletRegistration.jwtToken}`,
        },
        body: JSON.stringify({
          phone_number: phoneNumber,
          recaptcha_token: reCAPTCHAToken,
        }),
      });
      await request.json();

      onSuccess();
    } catch (error) {
      onError(error);
    }
  }
}

function disableButtons(buttons) {
  buttons.forEach((b) => {
    b.disabled = true;
  });
}

function enableButtons(buttons) {
  const t = window.setTimeout(() => {
    buttons.forEach((b) => {
      b.disabled = false;
    });

    clearTimeout(t);
  }, 1000);
}

document.addEventListener("DOMContentLoaded", function() {
  const form = document.getElementById("submitPhoneNumberForm");

  form.addEventListener("submit", function(event) {
    submitPhoneNumber(event);
  });
});

async function submitPhoneNumber(event) {
  event.preventDefault();
  const phoneNumberSectionEl = document.querySelector(
    "[data-section='phoneNumber']",
  );
  const passcodeSectionEl = document.querySelector("[data-section='passcode']");
  const errorNotificationEl = document.querySelector(
    "[data-section-error='phoneNumber']",
  );
  const phoneNumberEl = document.getElementById("phone_number");
  const reCAPTCHATokenEl = phoneNumberSectionEl.querySelector("#g-recaptcha-response")
  const buttonEls = phoneNumberSectionEl.querySelectorAll("[data-button]");

  if (!reCAPTCHATokenEl || !reCAPTCHATokenEl.value) {
    toggleErrorNotification(errorNotificationEl, "Error", "reCAPTCHA is required", true);
    return;
  }

  toggleErrorNotification(errorNotificationEl, "", "", false);

  if (
    phoneNumberEl &&
    reCAPTCHATokenEl &&
    phoneNumberSectionEl &&
    passcodeSectionEl &&
    errorNotificationEl
  ) {
    disableButtons(buttonEls);
    const phoneNumber = phoneNumberEl.value;
    const reCAPTCHAToken = reCAPTCHATokenEl.value;

    function showNextPage() {
      phoneNumberSectionEl.style.display = "none";
      reCAPTCHATokenEl.style.display = "none";
      passcodeSectionEl.style.display = "flex";
      enableButtons(buttonEls);
    }

    function showErrorMessage(error) {
      toggleErrorNotification(errorNotificationEl, "Error", error, true);
      enableButtons(buttonEls);
    }

    sendSms(phoneNumber, reCAPTCHAToken, showNextPage, showErrorMessage);
  }
}

document.addEventListener("DOMContentLoaded", function() {
  const form = document.getElementById("submitOtpForm");

  form.addEventListener("submit", function(event) {
    submitOtp(event);
  });
});

async function submitOtp(event) {
  event.preventDefault();

  const passcodeSectionEl = document.querySelector("[data-section='passcode']");
  const errorNotificationEl = document.querySelector(
    "[data-section-error='passcode']",
  );
  const successNotificationEl = document.querySelector(
    "[data-section-success='passcode']",
  );
  const phoneNumberEl = document.getElementById("phone_number");
  const otpEl = document.getElementById("otp");
  const verificationEl = document.getElementById("verification");

  const buttonEls = passcodeSectionEl.querySelectorAll("[data-button]");

  const reCAPTCHATokenEl = passcodeSectionEl.querySelector("#g-recaptcha-response-1");
  if (!reCAPTCHATokenEl || !reCAPTCHATokenEl.value) {
    toggleErrorNotification(errorNotificationEl, "Error", "reCAPTCHA is required", true);
    return;
  }

  if (
    phoneNumberEl &&
    otpEl &&
    verificationEl &&
    passcodeSectionEl &&
    errorNotificationEl
  ) {
    toggleErrorNotification(errorNotificationEl, "", "", false);
    toggleSuccessNotification(successNotificationEl, "", "", false);

    const phoneNumber = phoneNumberEl.value;
    const otp = otpEl.value;
    const verification = verificationEl.value;

    if (phoneNumber && otp && verification) {
      try {
        disableButtons(buttonEls);

        const response = await fetch("/wallet-registration/verification", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${WalletRegistration.jwtToken}`,
          },
          body: JSON.stringify({
            phone_number: phoneNumber,
            otp: otp,
            verification: verification,
            verification_type: "date_of_birth",
            recaptcha_token: reCAPTCHATokenEl.value,
          }),
        });

        if ([200, 201].includes(response.status)) {
          await response.json();

          const t = window.setTimeout(() => {
            location.reload();
            clearTimeout(t);
          }, 2000);
        } else {
          throw new Error("Something went wrong, please try again later.");
        }
      } catch (error) {
        enableButtons(buttonEls);
        toggleErrorNotification(errorNotificationEl, "Error", error, true);
        grecaptcha.reset(1);
      }
    }
  }
}

document.addEventListener("DOMContentLoaded", function() {
  const button = document.getElementById('resendSmsButton');

  button.addEventListener('click', function(event) {
    resendSms(event);
  });
});

async function resendSms() {
  const passcodeSectionEl = document.querySelector("[data-section='passcode']");
  const errorNotificationEl = document.querySelector(
    "[data-section-error='passcode']",
  );
  const successNotificationEl = document.querySelector(
    "[data-section-success='passcode']",
  );
  const buttonEls = passcodeSectionEl.querySelectorAll("[data-button]");
  const phoneNumberEl = document.getElementById("phone_number");
  const reCAPTCHATokenEl = passcodeSectionEl.querySelector("#g-recaptcha-response-1");

  if (!reCAPTCHATokenEl || !reCAPTCHATokenEl.value) {
    toggleErrorNotification(errorNotificationEl, "Error", "reCAPTCHA is required", true);
    return;
  }

  if ((passcodeSectionEl, errorNotificationEl, phoneNumberEl, reCAPTCHATokenEl)) {
    disableButtons(buttonEls);
    toggleErrorNotification(errorNotificationEl, "", "", false);
    toggleSuccessNotification(successNotificationEl, "", "", false);

    const phoneNumber = phoneNumberEl.value;
    const reCAPTCHAToken = reCAPTCHATokenEl.value;

    function showErrorMessage(error) {
      toggleErrorNotification(errorNotificationEl, "Error", error, true);
      enableButtons(buttonEls);
    }

    function showSuccessMessage() {
      toggleSuccessNotification(
        successNotificationEl,
        "New SMS sent",
        "You will receive a new one-time passcode",
        true,
      );
      enableButtons(buttonEls);
    }

    sendSms(phoneNumber, reCAPTCHAToken, showSuccessMessage, showErrorMessage);
    grecaptcha.reset(1);
  }
}

// Init
window.onload = async () => {
  WalletRegistration.jwtToken = getJwtToken();
};
