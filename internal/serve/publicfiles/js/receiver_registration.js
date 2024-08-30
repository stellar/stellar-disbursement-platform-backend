// Init
window.onload = async () => {
  WalletRegistration.jwtToken = getJwtToken();
  WalletRegistration.privacyPolicyLink = getPrivacyPolicyLink();
  WalletRegistration.intlTelInput = phoneNumberInit();
};

const ContactMethods = Object.freeze({
  PHONE_NUMBER: "phone_number",
  EMAIL: "email",
});

const CurrentStep = Object.freeze({
  SELECT_OTP_METHOD: "selectOtpMethod", // Step 1
  PHONE_NUMBER: "phoneNumber",          // Step 2 (w/ phone_number)
  EMAIL_ADDRESS: "emailAddress",        // Step 2 (w/ email)
  PASSCODE: "passcode",                 // Step 3
});

const VerificationField = Object.freeze({
  DATE_OF_BIRTH: "DATE_OF_BIRTH",
  YEAR_MONTH: "YEAR_MONTH",
  NATIONAL_ID_NUMBER: "NATIONAL_ID_NUMBER",
  PIN: "PIN",
});

const WalletRegistration = {
  jwtToken: "",
  intlTelInput: null,
  privacyPolicyLink: "",
  contactMethod: "",
  currentStep: CurrentStep.SELECT_OTP_METHOD,
  verificationField: "",

  setStep(step) {
    this.currentStep = step;
    switch (step) {
      case CurrentStep.PHONE_NUMBER:
        this.contactMethod = ContactMethods.PHONE_NUMBER;
        break;
      case CurrentStep.EMAIL_ADDRESS:
        this.contactMethod = ContactMethods.EMAIL;
        break;
    }

    const steps = Object.values(CurrentStep);
    steps.forEach((s) => {
      if (s === step) {
        document.querySelector(`[data-section='${s}']`).style.display = "flex";
      } else {
        document.querySelector(`[data-section='${s}']`).style.display = "none";
      }
    });
  },

  errorNotificationEl() {
    switch (this.currentStep) {
      case CurrentStep.PHONE_NUMBER:
      case CurrentStep.EMAIL_ADDRESS:
      case CurrentStep.PASSCODE:
        return document.querySelector("[data-section-error]");
    }
  },

  successNotificationEl() {
    if (this.currentStep === CurrentStep.PASSCODE) {
      return document.querySelector("[data-section-success]");
    }
  },

  toggleErrorNotification(title, message, isVisible) {
    const errorNotificationEl = this.errorNotificationEl();
    if (!errorNotificationEl) return;
    toggleErrorNotification(errorNotificationEl, title, message, isVisible);
  },

  toggleSuccessNotification(title, message, isVisible) {
    const successNotificationEl = this.successNotificationEl();
    if (!successNotificationEl) return;
    toggleSuccessNotification(successNotificationEl, title, message, isVisible);
  },

  getRecaptchaToken() {
    switch (this.currentStep) {
      case CurrentStep.EMAIL_ADDRESS:
        return document.querySelector("#g-recaptcha-response").value;
      case CurrentStep.PHONE_NUMBER:
        return document.querySelector("#g-recaptcha-response-1").value;
      case CurrentStep.PASSCODE:
        return document.querySelector("#g-recaptcha-response-2").value;
    }
  },

  getSectionEl() {
    switch (this.currentStep) {
      case CurrentStep.PHONE_NUMBER:
        return document.querySelector("[data-section='phoneNumber']");
      case CurrentStep.EMAIL_ADDRESS:
        return document.querySelector("[data-section='emailAddress']");
      case CurrentStep.PASSCODE:
        return document.querySelector("[data-section='passcode']");
    }
  },

  toggleButtonsEnabled(isEnabled) {
    const sectionEl = this.getSectionEl();
    const buttonEls = sectionEl.querySelectorAll("[data-button]");
    const t = window.setTimeout(() => {
      buttonEls.forEach((b) => {
        b.disabled = !isEnabled;
      });

      clearTimeout(t);
    }, isEnabled ? 1000 : 0);
  },

  ////////////////////////////////////////// ⬆️ NEW CODE ⬆️ //////////////////////////////////////////
  getContactValue() {
    switch (this.contactMethod) {
      case ContactMethods.PHONE_NUMBER:
        return this.intlTelInput.getNumber().trim();
      case ContactMethods.EMAIL:
        return document.querySelector("#email_address").value.trim();
    }
  },

  validateContactValue() {
    const contactValue = this.getContactValue();
    if (contactValue === "") {
      this.toggleErrorNotification("Error", "Contact information is required", true);
      return -1;
    }

    switch (this.contactMethod) {
      case ContactMethods.PHONE_NUMBER:
        if (!WalletRegistration.intlTelInput.isPossibleNumber()) {
          this.toggleErrorNotification("Error", "Entered phone number is not valid", true);
          return -1;
        }
        break;
      case ContactMethods.EMAIL:
        const isValidEmail = (email) => {
          const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
          return emailRegex.test(email);
        };

        if (!isValidEmail(contactValue)) {
          this.toggleErrorNotification("Error", "Entered email is not valid", true);
          return -1;
        }
        break;
    }

    this.toggleErrorNotification("", "", false);
    return 0;
  }
};

function getJwtToken() {
  const tokenEl = document.querySelector("[data-jwt-token]");

  if (tokenEl) {
    return tokenEl.innerHTML;
  }
}

function getPrivacyPolicyLink() {
  const linkEl = document.querySelector("[data-privacy-policy-link]");

  if (linkEl) {
    return linkEl.innerHTML;
  }
}

document.addEventListener("DOMContentLoaded", function () {
  const footer = document.getElementById("WalletRegistration__PrivacyPolicy");

  if (WalletRegistration.privacyPolicyLink == "") {
    footer.style = "display: none"
  }
});

// ------------------------------ START: STEP 1 ------------------------------
document.addEventListener("DOMContentLoaded", function () {
  const otpMethodForm = document.getElementById("selectOtpMethodForm");

  otpMethodForm.addEventListener("submit", function (event) {
    handleSelectOtp(event);
  });
});

function handleSelectOtp(event) {
  event.preventDefault();
  const selectedMethod = document.querySelector('input[name="otp_method"]:checked').value;
  WalletRegistration.setStep(selectedMethod);
}
// ------------------------------ END: STEP 1 ------------------------------

// ------------------------------ START: STEP 2 ------------------------------
document.addEventListener("DOMContentLoaded", function () {
  // email submitted listener
  const submitEmailForm = document.getElementById("submitEmailForm");
  submitEmailForm.addEventListener("submit", function (event) {
    event.preventDefault();
    submitContactInfo();
  });

  // phone number submitted listener
  const submitPhoneForm = document.getElementById("submitPhoneNumberForm");
  submitPhoneForm.addEventListener("submit", function (event) {
    event.preventDefault();
    submitContactInfo();
  });
});

async function submitContactInfo() {
  if (![CurrentStep.PHONE_NUMBER, CurrentStep.EMAIL_ADDRESS].includes(WalletRegistration.currentStep)) {
    alert("Invalid step to submit contact information: " + WalletRegistration.currentStep);
    return;
  }

  const reCAPTCHAToken = WalletRegistration.getRecaptchaToken();
  if (!reCAPTCHAToken) {
    WalletRegistration.toggleErrorNotification("Error", "reCAPTCHA is required", true);
    return;
  }

  WalletRegistration.toggleErrorNotification("", "", false);
  WalletRegistration.toggleButtonsEnabled(false);
  if (WalletRegistration.validateContactValue() === -1) {
    return;
  }

  function showNextPage(verificationField) {
    const verificationFieldTitle = document.querySelector("label[for='verification']");
    const verificationFieldInput = document.querySelector("#verification");
    WalletRegistration.verificationField = verificationField;

    switch (verificationField) {
      case VerificationField.DATE_OF_BIRTH:
        verificationFieldTitle.textContent = "Date of birth";
        verificationFieldInput.name = "date_of_birth";
        verificationFieldInput.type = "date";
        break;
      case VerificationField.YEAR_MONTH:
        verificationFieldTitle.textContent = "Date of birth (Year/Month)";
        verificationFieldInput.name = "year_month";
        verificationFieldInput.type = "month";
        break;
      case VerificationField.NATIONAL_ID_NUMBER:
        verificationFieldTitle.textContent = "National ID number";
        verificationFieldInput.name = "national_id_number";
        verificationFieldInput.type = "text";
        break;
      case VerificationField.PIN:
        verificationFieldTitle.textContent = "Pin";
        verificationFieldInput.name = "pin";
        verificationFieldInput.type = "text";
        break;
    }

    WalletRegistration.setStep(CurrentStep.PASSCODE);
    WalletRegistration.toggleButtonsEnabled(true);
  }

  function showErrorMessage(error) {
    WalletRegistration.toggleErrorNotification("Error", error, true);
    WalletRegistration.toggleButtonsEnabled(true);
  }

  sendOtp(showNextPage, showErrorMessage);
}

// ------------------------------ END: STEP 2 ------------------------------

// ------------------------------ START: STEP 3 ------------------------------
document.addEventListener("DOMContentLoaded", function () {
  const form = document.getElementById("submitOtpForm");

  form.addEventListener("submit", function (event) {
    submitOtp(event);
  });
});

async function submitOtp(event) {
  event.preventDefault();

  const reCAPTCHAToken = WalletRegistration.getRecaptchaToken();
  if (!reCAPTCHAToken) {
    WalletRegistration.toggleErrorNotification("Error", "reCAPTCHA is required", true);
    return;
  }

  const contactMethod = WalletRegistration.contactMethod;
  const contactValue = WalletRegistration.getContactValue();
  const otp = document.getElementById("otp").value;
  const verificationFieldValue = document.getElementById("verification").value;

  if (contactMethod && contactValue && otp && verificationFieldValue) {
    WalletRegistration.toggleErrorNotification("", "", false);
    WalletRegistration.toggleSuccessNotification("", "", false);

    try {
      WalletRegistration.toggleButtonsEnabled(false);

      const response = await fetch("/wallet-registration/verification", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${WalletRegistration.jwtToken}`,
        },
        body: JSON.stringify({
          [contactMethod]: contactValue,
          otp: otp,
          recaptcha_token: reCAPTCHAToken,
          verification_type: WalletRegistration.verificationField,
          verification: verificationFieldValue,
        }),
      });

      if (Math.floor(response.status / 100) === 2) {
        await response.json();
        const t = window.setTimeout(() => {
          location.reload();
          clearTimeout(t);
        }, 2000);
      } else if (response.status === 400) {
        const data = await response.json();
        const errorMessage = data.error || "Something went wrong with your request, please try again later.";
        throw new Error(errorMessage);
      } else {
        throw new Error(`Something went wrong, please try again later (status code: ${response.status}).`);
      }
    } catch (error) {
      WalletRegistration.toggleButtonsEnabled(true);
      WalletRegistration.toggleErrorNotification("Error", error, true);
      grecaptcha.reset(1);
    }
  }
}

document.addEventListener("DOMContentLoaded", function () {
  const button = document.getElementById("resendOtpButton");

  button.addEventListener("click", function (event) {
    event.preventDefault();
    resendOtp();
  });
});

async function resendOtp() {
  const reCAPTCHAToken = WalletRegistration.getRecaptchaToken();
  if (!reCAPTCHAToken) {
    WalletRegistration.toggleErrorNotification("Error", "reCAPTCHA is required", true);
    return;
  }

  const contactValue = WalletRegistration.getContactValue();
  if (!contactValue) {
    WalletRegistration.toggleErrorNotification("Error", "Contact information is required", true);
    return;
  }

  WalletRegistration.toggleButtonsEnabled(false);
  WalletRegistration.toggleErrorNotification("", "", false);
  WalletRegistration.toggleSuccessNotification("", "", false);

  function showErrorMessage(error) {
    WalletRegistration.toggleErrorNotification("Error", error, true);
    WalletRegistration.toggleButtonsEnabled(true);
  }

  function showSuccessMessage() {
    WalletRegistration.toggleSuccessNotification("New OTP sent", "You will receive a new one-time passcode", true);
    WalletRegistration.toggleButtonsEnabled(true);
  }

  sendOtp(showSuccessMessage, showErrorMessage);
  grecaptcha.reset(1);
}
// ------------------------------ END: STEP 3 ------------------------------

// ------------------------------ START: UTILITY FUNCTIONS ------------------------------
// Phone number input
// https://github.com/jackocnr/intl-tel-input
function phoneNumberInit() {
  const phoneNumberInput = document.querySelector("#phone_number");

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
  ["change", "keyup"].forEach((event) => {
    phoneNumberInput.addEventListener(event, () => {
      if (WalletRegistration.errorNotificationEl().style.display !== "none") {
        WalletRegistration.toggleErrorNotification("", "", false);
        WalletRegistration.toggleButtonsEnabled(true);
      }
    });
  });

  return intlTelInput;
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

async function sendOtp(onSuccess, onError) {
  const reqPayload = {
    [WalletRegistration.contactMethod]: WalletRegistration.getContactValue(),
    recaptcha_token: WalletRegistration.getRecaptchaToken(),
  };

  try {
    const response = await fetch("/wallet-registration/otp", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${WalletRegistration.jwtToken}`,
      },
      body: JSON.stringify(reqPayload),
    });

    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || "Something went wrong, please try again later.");
    }

    onSuccess(data.verification_field);;
  } catch (error) {
    onError(error);
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
// ------------------------------ END: UTILITY FUNCTIONS ------------------------------