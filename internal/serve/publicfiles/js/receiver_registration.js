const WalletRegistration = {
  jwtToken: "",
  intlTelInput: null,
  phoneNumberErrorEl: null,
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
      const resp = await request.json();

      onSuccess(resp.verification_field);
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

document.addEventListener("DOMContentLoaded", function () {
  const form = document.getElementById("submitPhoneNumberForm");

  form.addEventListener("submit", function (event) {
    submitPhoneNumber(event);
  });
});

async function submitPhoneNumber(event) {
  event.preventDefault();
  const phoneNumberEl = document.querySelector("#phone_number");
  const phoneNumberSectionEl = document.querySelector(
    "[data-section='phoneNumber']"
  );
  const passcodeSectionEl = document.querySelector("[data-section='passcode']");
  const errorNotificationEl = WalletRegistration.phoneNumberErrorEl;
  const reCAPTCHATokenEl = phoneNumberSectionEl.querySelector(
    "#g-recaptcha-response"
  );
  const buttonEls = phoneNumberSectionEl.querySelectorAll("[data-button]");
  const verificationFieldTitle = document.querySelector("label[for='verification']");
  const verificationFieldInput = document.querySelector("#verification");

  if (!reCAPTCHATokenEl || !reCAPTCHATokenEl.value) {
    toggleErrorNotification(
      errorNotificationEl,
      "Error",
      "reCAPTCHA is required",
      true
    );
    return;
  }

  toggleErrorNotification(errorNotificationEl, "", "", false);

  if (
    WalletRegistration.intlTelInput &&
    reCAPTCHATokenEl &&
    phoneNumberSectionEl &&
    passcodeSectionEl &&
    errorNotificationEl
  ) {
    disableButtons(buttonEls);
    const phoneNumber = WalletRegistration.intlTelInput.getNumber();
    const reCAPTCHAToken = reCAPTCHATokenEl.value;

    if (
      phoneNumberEl.value.trim() &&
      !WalletRegistration.intlTelInput.isPossibleNumber()
    ) {
      toggleErrorNotification(
        errorNotificationEl,
        "Error",
        "Entered phone number is not valid",
        true
      );
      return;
    }

    function showNextPage(verificationField) {
      verificationFieldInput.type = "text";
      if(verificationField === "DATE_OF_BIRTH") {
        verificationFieldTitle.textContent = "Date of birth";
        verificationFieldInput.name = "date_of_birth";
        verificationFieldInput.type = "date";
      }
      else if(verificationField === "NATIONAL_ID_NUMBER") {
        verificationFieldTitle.textContent = "National ID number";
        verificationFieldInput.name = "national_id_number";
      }
      else if(verificationField === "PIN") {
        verificationFieldTitle.textContent = "Pin";
        verificationFieldInput.name = "pin";
      }

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

document.addEventListener("DOMContentLoaded", function () {
  const form = document.getElementById("submitOtpForm");

  form.addEventListener("submit", function (event) {
    submitOtp(event);
  });
});

async function submitOtp(event) {
  event.preventDefault();

  const passcodeSectionEl = document.querySelector("[data-section='passcode']");
  const errorNotificationEl = document.querySelector(
    "[data-section-error='passcode']"
  );
  const successNotificationEl = document.querySelector(
    "[data-section-success='passcode']"
  );
  const otpEl = document.getElementById("otp");
  const verificationEl = document.getElementById("verification");
  const verificationField = verificationEl.getAttribute("name");

  const buttonEls = passcodeSectionEl.querySelectorAll("[data-button]");

  const reCAPTCHATokenEl = passcodeSectionEl.querySelector(
    "#g-recaptcha-response-1"
  );
  if (!reCAPTCHATokenEl || !reCAPTCHATokenEl.value) {
    toggleErrorNotification(
      errorNotificationEl,
      "Error",
      "reCAPTCHA is required",
      true
    );
    return;
  }

  if (
    WalletRegistration.intlTelInput &&
    otpEl &&
    verificationEl &&
    passcodeSectionEl &&
    errorNotificationEl
  ) {
    toggleErrorNotification(errorNotificationEl, "", "", false);
    toggleSuccessNotification(successNotificationEl, "", "", false);

    const phoneNumber = WalletRegistration.intlTelInput.getNumber();
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
            verification_type: verificationField,
            recaptcha_token: reCAPTCHATokenEl.value,
          }),
        });

        if ([200, 201].includes(response.status)) {
          await response.json();

          const t = window.setTimeout(() => {
            location.reload();
            clearTimeout(t);
          }, 2000);
        } else if (response.status === 400) {
          const data = await response.json();
          const errorMessage = data.error || "Something went wrong, please try again later.";
          throw new Error(errorMessage);
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

document.addEventListener("DOMContentLoaded", function () {
  const button = document.getElementById("resendSmsButton");

  button.addEventListener("click", function (event) {
    resendSms(event);
  });
});

async function resendSms() {
  const passcodeSectionEl = document.querySelector("[data-section='passcode']");
  const errorNotificationEl = document.querySelector(
    "[data-section-error='passcode']"
  );
  const successNotificationEl = document.querySelector(
    "[data-section-success='passcode']"
  );
  const buttonEls = passcodeSectionEl.querySelectorAll("[data-button]");
  const reCAPTCHATokenEl = passcodeSectionEl.querySelector(
    "#g-recaptcha-response-1"
  );

  if (!reCAPTCHATokenEl || !reCAPTCHATokenEl.value) {
    toggleErrorNotification(
      errorNotificationEl,
      "Error",
      "reCAPTCHA is required",
      true
    );
    return;
  }

  if (
    (passcodeSectionEl,
    errorNotificationEl,
    WalletRegistration.intlTelInput,
    reCAPTCHATokenEl)
  ) {
    disableButtons(buttonEls);
    toggleErrorNotification(errorNotificationEl, "", "", false);
    toggleSuccessNotification(successNotificationEl, "", "", false);

    const phoneNumber = WalletRegistration.intlTelInput.getNumber();
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
        true
      );
      enableButtons(buttonEls);
    }

    sendSms(phoneNumber, reCAPTCHAToken, showSuccessMessage, showErrorMessage);
    grecaptcha.reset(1);
  }
}

function resetNumberInputError(buttonEls) {
  if (
    WalletRegistration.phoneNumberErrorEl &&
    WalletRegistration.phoneNumberErrorEl.style.display !== "none"
  ) {
    toggleErrorNotification(
      WalletRegistration.phoneNumberErrorEl,
      "",
      "",
      false
    );
    enableButtons(buttonEls);
  }
}

// Phone number input
// https://github.com/jackocnr/intl-tel-input
function phoneNumberInit() {
  const phoneNumberInput = document.querySelector("#phone_number");
  const phoneNumberSectionEl = document.querySelector(
    "[data-section='phoneNumber']"
  );
  const buttonEls = phoneNumberSectionEl.querySelectorAll("[data-button]");

  const intlTelInput = window.intlTelInput(phoneNumberInput, {
    utilsScript: "/static/js/intl-tel-input-v18.2.1-utils.min.js",
    separateDialCode: true,
    preferredCountries: [],
    // Excluding Cuba, Iran, North Korea, and Syria
    excludeCountries: ["cu", "ir", "kp", "sy"],
    // Setting default country based on user's IP address
    initialCountry: "auto",
    geoIpLookup: (callback) => {
      fetch("https://ipapi.co/json")
        .then((res) => res.json())
        .then((data) => callback(data.country_code))
        .catch(() => callback(""));
    },
  });

  // Clear phone number error message
  phoneNumberInput.addEventListener("change", () =>
    resetNumberInputError(buttonEls)
  );
  phoneNumberInput.addEventListener("keyup", () =>
    resetNumberInputError(buttonEls)
  );

  return intlTelInput;
}

// Init
window.onload = async () => {
  WalletRegistration.jwtToken = getJwtToken();
  WalletRegistration.intlTelInput = phoneNumberInit();
  WalletRegistration.phoneNumberErrorEl = document.querySelector(
    "[data-section-error='phoneNumber']"
  );
};
