// ------------------------------ START: ENUMS ------------------------------
const ContactMethods = Object.freeze({
  PHONE_NUMBER: "phone_number",
  EMAIL: "email",
});

const CurrentSection = Object.freeze({
  SELECT_OTP_METHOD: "selectOtpMethod", // SECTION 1
  PHONE_NUMBER: "phoneNumber",          // SECTION 2.1 (w/ phone_number)
  EMAIL_ADDRESS: "emailAddress",        // SECTION 2.2 (w/ email)
  PASSCODE: "passcode",                 // SECTION 3
});

const VerificationField = Object.freeze({
  DATE_OF_BIRTH: "DATE_OF_BIRTH",
  YEAR_MONTH: "YEAR_MONTH",
  NATIONAL_ID_NUMBER: "NATIONAL_ID_NUMBER",
  PIN: "PIN",
});
// ------------------------------ END: ENUMS ------------------------------


// ------------------------------ START: GLOBAL VARIABLES AND METHODS ------------------------------
const reCAPTCHAWidgets = {};

function resetReCAPTCHA() {
  Object.values(reCAPTCHAWidgets).forEach(widgetId => grecaptcha.reset(widgetId));
}

const WalletRegistration = {
  jwtToken: "",
  intlTelInput: null,
  privacyPolicyLink: "",
  contactMethod: "",
  currentSection: CurrentSection.SELECT_OTP_METHOD,
  verificationField: "",

  setSection(section) {
    this.currentSection = section;
    switch (section) {
      case CurrentSection.PHONE_NUMBER:
        this.contactMethod = ContactMethods.PHONE_NUMBER;
        break;
      case CurrentSection.EMAIL_ADDRESS:
        this.contactMethod = ContactMethods.EMAIL;
        break;
    }

    Object.values(CurrentSection).forEach((s) => {
      const sectionEl = document.querySelector(`[data-section='${s}']`);
      if (sectionEl) sectionEl.style.display = s === section ? "flex" : "none";
    });
  },

  errorNotificationEl() {
    return document.querySelector("[data-section-error]");
  },

  successNotificationEl() {
    return document.querySelector("[data-section-success]");
  },

  toggleErrorNotification(title, message, isVisible) {
    const errorNotificationEl = this.errorNotificationEl();
    toggleNotification("error", { parentEl: errorNotificationEl, title, message, isVisible });
  },

  toggleSuccessNotification(title, message, isVisible) {
    const successNotificationEl = this.successNotificationEl();
    toggleNotification("success", { parentEl: successNotificationEl, title, message, isVisible });
  },

  getRecaptchaToken() {
    const tokenSelectorMap = {
      [CurrentSection.EMAIL_ADDRESS]: "#g-recaptcha-response",
      [CurrentSection.PHONE_NUMBER]: "#g-recaptcha-response-1",
      [CurrentSection.PASSCODE]: "#g-recaptcha-response-2",
    };

    const recaptchaEl = document.querySelector(tokenSelectorMap[this.currentSection]);
    return recaptchaEl?.value || "";
  },

  getSectionEl() {
    return document.querySelector(`[data-section='${this.currentSection}']`);
  },

  toggleButtonsEnabled(isEnabled) {
    const sectionEl = this.getSectionEl();
    const buttonEls = sectionEl?.querySelectorAll("[data-button]");
    if (!buttonEls) return;
    const t = window.setTimeout(() => {
      buttonEls.forEach((b) => {
        b.disabled = !isEnabled;
      });

      clearTimeout(t);
    }, isEnabled ? 1000 : 0);
  },

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
    if (!contactValue) {
      this.toggleErrorNotification("Error", "Contact information is required", true);
      return -1;
    }

    if (this.contactMethod === ContactMethods.PHONE_NUMBER) {
      if (!this.intlTelInput.isPossibleNumber()) {
        this.toggleErrorNotification("Error", "Entered phone number is not valid", true);
        return -1;
      }
    } else if (this.contactMethod === ContactMethods.EMAIL) {
      const isValidEmail = (email) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
      if (!isValidEmail(contactValue)) {
        this.toggleErrorNotification("Error", "Entered email is not valid", true);
        return -1;
      }
    }

    this.toggleErrorNotification("", "", false);
    return 0;
  }
};
// ------------------------------ END: GLOBAL VARIABLES AND METHODS ------------------------------


// ------------------------------ START: INITIALIZATION ------------------------------
window.onload = () => {
  WalletRegistration.jwtToken = document.querySelector("#jwt-token").dataset.jwtToken
  WalletRegistration.privacyPolicyLink = document.querySelector("[data-privacy-policy-link]")?.innerHTML || "";
  WalletRegistration.intlTelInput = phoneNumberInit();

  // Render reCAPTCHA instances and store their widget IDs
  const siteKey = document.querySelector("#recaptcha-site-key").dataset.sitekey;
  reCAPTCHAWidgets.email = grecaptcha.render('g-recaptcha-email', { sitekey: siteKey });
  reCAPTCHAWidgets.phone = grecaptcha.render('g-recaptcha-phone', { sitekey: siteKey });
  reCAPTCHAWidgets.passcode = grecaptcha.render('g-recaptcha-passcode', { sitekey: siteKey });
};

// Phone number input (ref: https://github.com/jackocnr/intl-tel-input)
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
  const errorNotificationEl = WalletRegistration.errorNotificationEl();
  ["change", "keyup"].forEach((event) => {
    phoneNumberInput.addEventListener(event, () => {
      if (errorNotificationEl.style.display !== "none") {
        WalletRegistration.toggleErrorNotification("", "", false);
        WalletRegistration.toggleButtonsEnabled(true);
      }
    });
  });

  return intlTelInput;
}

document.addEventListener("DOMContentLoaded", function () {
  // Hide Privacy Policy Link if not provided
  const footer = document.getElementById("WalletRegistration__PrivacyPolicy");
  if (!WalletRegistration.privacyPolicyLink) {
    footer.style.display = "none";
  }

  // SECTION 1: Setup OTP Method Form
  const otpMethodForm = document.getElementById("selectOtpMethodForm");
  otpMethodForm?.addEventListener("change", () => {
    WalletRegistration.toggleErrorNotification("", "", false);
  });
  otpMethodForm?.addEventListener("submit", (event) => {
    event.preventDefault();
    handleOtpSelected();
  });

  // SECTION 2: Setup Email and Phone Number Forms
  ["submitEmailForm", "submitPhoneNumberForm"].forEach((formId) => {
    document.getElementById(formId)?.addEventListener("submit", (event) => {
      event.preventDefault();
      handleContactInfoSubmitted();
    });
  });

  // SECTION 3: Setup OTP Form
  document.getElementById("submitVerificationForm")?.addEventListener("submit", (event) => {
    event.preventDefault();
    handleVerificationInfoSubmitted();
  });

  // SECTION 3: Setup Resend OTP Button
  document.getElementById("resendOtpButton")?.addEventListener("click", (event) => {
    event.preventDefault();
    handleResendOtpClicked();
  });
});


// ------------------------------ START: SECTION 1 ------------------------------
function handleOtpSelected() {
  const selectedMethod = document.querySelector('input[name="otp_method"]:checked')?.value;
  if (!selectedMethod) {
    WalletRegistration.toggleErrorNotification("Error", "Please select a contact method to receive your OTP", true);
    return;
  }
  WalletRegistration.setSection(selectedMethod);
}
// ------------------------------ END: SECTION 1 ------------------------------


// ------------------------------ START: SECTION 2 ------------------------------
async function handleContactInfoSubmitted() {
  if (![CurrentSection.PHONE_NUMBER, CurrentSection.EMAIL_ADDRESS].includes(WalletRegistration.currentSection)) {
    alert("Invalid section to submit contact information: " + WalletRegistration.currentSection);
    return;
  }

  const reCAPTCHAToken = WalletRegistration.getRecaptchaToken();
  if (!reCAPTCHAToken) {
    WalletRegistration.toggleErrorNotification("Error", "reCAPTCHA is required", true);
    return;
  }

  WalletRegistration.toggleErrorNotification("", "", false);
  WalletRegistration.toggleButtonsEnabled(false);
  if (WalletRegistration.validateContactValue() === -1) return;

  function showNextPage(verificationField) {
    const verificationFieldTitle = document.querySelector("label[for='verification']");
    const verificationFieldInput = document.querySelector("#verification");
    WalletRegistration.verificationField = verificationField;

    const inputFeldConfigMap = {
      [VerificationField.DATE_OF_BIRTH]: { name: "date_of_birth", type: "date", label: "Date of birth" },
      [VerificationField.YEAR_MONTH]: { name: "year_month", type: "month", label: "Date of birth (Year/Month)" },
      [VerificationField.NATIONAL_ID_NUMBER]: { name: "national_id_number", type: "text", label: "National ID number" },
      [VerificationField.PIN]: { name: "pin", type: "text", label: "Pin" },
    };

    const inputFieldConfig = inputFeldConfigMap[verificationField];
    if (inputFieldConfig) {
      verificationFieldTitle.textContent = inputFieldConfig.label;
      verificationFieldInput.name = inputFieldConfig.name;
      verificationFieldInput.type = inputFieldConfig.type;
    }

    WalletRegistration.setSection(CurrentSection.PASSCODE);
    WalletRegistration.toggleButtonsEnabled(true);
  }

  function showErrorMessage(error) {
    WalletRegistration.toggleErrorNotification("Error", error, true);
    WalletRegistration.toggleButtonsEnabled(true);
  }

  sendOtp(showNextPage, showErrorMessage);
}
// ------------------------------ END: SECTION 2 ------------------------------


// ------------------------------ START: SECTION 3 ------------------------------
async function handleVerificationInfoSubmitted() {
  const reCAPTCHAToken = WalletRegistration.getRecaptchaToken();
  if (!reCAPTCHAToken) {
    WalletRegistration.toggleErrorNotification("Error", "reCAPTCHA is required", true);
    return;
  }

  const contactMethod = WalletRegistration.contactMethod;
  const contactValue = WalletRegistration.getContactValue();
  const otp = document.getElementById("otp").value;
  const verificationFieldValue = document.getElementById("verification").value;
  if (!contactMethod || !contactValue || !otp || !verificationFieldValue) {
    const errMessage = `Missing one of the required fields: ${{ contactMethod, contactValue, otp, verificationFieldValue }}`;
    WalletRegistration.toggleErrorNotification("Error", errMessage, true);
    return;
  }

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
        verification_field: WalletRegistration.verificationField,
        verification: verificationFieldValue,
      }),
    });

    if (Math.floor(response.status / 100) === 2) {
      await response.json();
      setTimeout(() => {
        location.reload();
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
    resetReCAPTCHA();
  }
}

async function handleResendOtpClicked() {
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
  resetReCAPTCHA();
}
// ------------------------------ END: SECTION 3 ------------------------------


// ------------------------------ START: UTILITY FUNCTIONS ------------------------------
function toggleNotification(type, { parentEl, title, message, isVisible }) {
  const titleEl = parentEl.querySelector(`[data-section-${type}-title]`);
  const messageEl = parentEl.querySelector(`[data-section-${type}-message`);

  if (titleEl && messageEl) {
    parentEl.style.display = isVisible ? "flex" : "none";
    titleEl.innerHTML = isVisible ? title : "";
    messageEl.innerHTML = isVisible ? message : "";
  }
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

    onSuccess(data.verification_field);
  } catch (error) {
    onError(error);
  }
}
// ------------------------------ END: UTILITY FUNCTIONS ------------------------------
