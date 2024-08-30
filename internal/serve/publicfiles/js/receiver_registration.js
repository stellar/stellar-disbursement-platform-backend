const ContactMethods = Object.freeze({
  PHONE_NUMBER: Symbol("phone"),
  EMAIL: Symbol("email")
});

const WalletRegistration = {
  jwtToken: "",
  intlTelInput: null,
  contactInfoErrorEl: null,
  privacyPolicyLink: "",
  contactMethod: "",
  
  getContactSectionEl() {
    if (this.contactMethod === ContactMethods.PHONE_NUMBER) {
      return document.querySelector("[data-section='phoneNumber']");
    } else if (this.contactMethod === ContactMethods.EMAIL) {
      return document.querySelector("[data-section='emailAddress']");
    }
  },

  getContactSectionRecaptchaEl() {
    const contactSectionEl = this.getContactSectionEl();
    if (this.contactMethod === ContactMethods.PHONE_NUMBER) {
      return contactSectionEl.querySelector("#g-recaptcha-response-1");
    } else if (this.contactMethod === ContactMethods.EMAIL) {
      return contactSectionEl.querySelector("#g-recaptcha-response");
    }
  },

  getContactValue() {
    if (this.contactMethod === ContactMethods.PHONE_NUMBER) {
      return WalletRegistration.intlTelInput.getNumber().trim();
    } else if (this.contactMethod === ContactMethods.EMAIL) {
      return document.querySelector("#email_address").value.trim();
    }
  },

  validateContactValue() {
    const contactValue = this.getContactValue();
    const errorNotificationEl = this.contactInfoErrorEl;

    if (contactValue === "") {
      toggleErrorNotification(
        errorNotificationEl,
        "Error",
        "Contact information is required",
        true
      );
      return -1;
    }

    if (this.contactMethod === ContactMethods.PHONE_NUMBER) {
      if (!WalletRegistration.intlTelInput.isPossibleNumber()) {
        toggleErrorNotification(
          errorNotificationEl,
          "Error",
          "Entered phone number is not valid",
          true
        );
        return -1;
      }
    } else if (this.contactMethod === ContactMethods.EMAIL) {
      const isValidEmail = (email) => {
        const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
        return emailRegex.test(email);
      };

      if (!isValidEmail(contactValue)) {
        toggleErrorNotification(
          errorNotificationEl,
          "Error",
          "Entered email is not valid",
          true
        );
        return -1;
      }
    }

    toggleErrorNotification(errorNotificationEl, "", "", false);
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

document.addEventListener("DOMContentLoaded", function () {
  const otpMethodForm = document.getElementById("selectOtpMethodForm");

  otpMethodForm.addEventListener("submit", function (event) {
    selectOtpMethod(event);
  });
});

function selectOtpMethod(event) {
  event.preventDefault();

  const selectedMethod = document.querySelector(
    'input[name="otp_method"]:checked'
  ).value;

  const selectOtpMethodSection = document.querySelector(
    "[data-section='selectOtpMethod']"
  );
  selectOtpMethodSection.style.display = "none";

  if (selectedMethod === "phone") {
    WalletRegistration.contactMethod = ContactMethods.PHONE_NUMBER;
    WalletRegistration.intlTelInput = phoneNumberInit();
  } else if (selectedMethod === "email") {
    WalletRegistration.contactMethod = ContactMethods.EMAIL;
  }

  WalletRegistration.getContactSectionEl().style.display = "flex";
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

async function sendOtp(phoneNumber, reCAPTCHAToken, onSuccess, onError) {
  if (phoneNumber && reCAPTCHAToken) {
    try {
      const response = await fetch("/wallet-registration/otp", {
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

      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error || "Something went wrong, please try again later.");
      }

      onSuccess(data.verification_field);;
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

// email submitted listener
document.addEventListener("DOMContentLoaded", function () {
  const form = document.getElementById("submitEmailForm");

  form.addEventListener("submit", function (event) {
    event.preventDefault();
    alert("Click submit with email: ", WalletRegistration.getContactValue());
  //   const emailAddressEl = document.querySelector("#email_address");
  //   const emailAddressSectionEl = document.querySelector(
  //     "[data-section='emailAddress']"
  //   );

  //   const validateEmail = (email) => {
  //     if (
  //       email &&
  //       !WalletRegistration.intlTelInput.isPossibleNumber()
  //     ) {
  //       toggleErrorNotification(
  //         errorNotificationEl,
  //         "Error",
  //         "Entered email is not valid",
  //         true
  //       );
  //       return -1;
  //     }

  //     return 0
  //   };

  //   submitContactForm(event, emailAddressEl, emailAddressSectionEl, validateEmail);
  });
});

// async function submitEmail() {
//   const emailAddressEl = document.querySelector("#email_address");
//   const emailAddressSectionEl = document.querySelector(
//     "[data-section='emailAddress']"
//   );
//   const passcodeSectionEl = document.querySelector("[data-section='passcode']");
//   const errorNotificationEl = WalletRegistration.contactInfoErrorEl;
//   const reCAPTCHATokenEl = emailAddressSectionEl.querySelector(
//     "#g-recaptcha-response"
//   );
// }

// phone number submitted listener
document.addEventListener("DOMContentLoaded", function () {
  const form = document.getElementById("submitPhoneNumberForm");

  form.addEventListener("submit", function (event) {
    event.preventDefault();
    submitPhoneNumber();
  });
});

async function submitPhoneNumber() {
  const errorNotificationEl = WalletRegistration.contactInfoErrorEl;
  const reCAPTCHATokenEl = WalletRegistration.getContactSectionRecaptchaEl();
  const reCAPTCHAToken = reCAPTCHATokenEl.value;
  if (!reCAPTCHATokenEl || !reCAPTCHAToken) {
    toggleErrorNotification(
      errorNotificationEl,
      "Error",
      "reCAPTCHA is required",
      true
    );
    return;
  }

  toggleErrorNotification(errorNotificationEl, "", "", false);
  
  const passcodeSectionEl = document.querySelector("[data-section='passcode']");
  
  const sectionEl = WalletRegistration.getContactSectionEl();
  const buttonEls = sectionEl.querySelectorAll("[data-button]");
  const verificationFieldTitle = document.querySelector("label[for='verification']");
  const verificationFieldInput = document.querySelector("#verification");

  if (
    reCAPTCHATokenEl &&
    sectionEl &&
    passcodeSectionEl &&
    errorNotificationEl
  ) {
    disableButtons(buttonEls);

    if (WalletRegistration.validateContactValue() === -1) {
      return;
    }

    function showNextPage(verificationField) {
      verificationFieldInput.type = "text";
      if (verificationField === "DATE_OF_BIRTH") {
        verificationFieldTitle.textContent = "Date of birth";
        verificationFieldInput.name = "date_of_birth";
        verificationFieldInput.type = "date";
      }
      else if (verificationField === "YEAR_MONTH") {
        verificationFieldTitle.textContent = "Date of birth (Year/Month)";
        verificationFieldInput.name = "year_month";
        verificationFieldInput.type = "month";
      }
      else if (verificationField === "NATIONAL_ID_NUMBER") {
        verificationFieldTitle.textContent = "National ID number";
        verificationFieldInput.name = "national_id_number";
      }
      else if (verificationField === "PIN") {
        verificationFieldTitle.textContent = "Pin";
        verificationFieldInput.name = "pin";
      }

      sectionEl.style.display = "none";
      reCAPTCHATokenEl.style.display = "none";
      passcodeSectionEl.style.display = "flex";
      enableButtons(buttonEls);
    }

    function showErrorMessage(error) {
      toggleErrorNotification(errorNotificationEl, "Error", error, true);
      enableButtons(buttonEls);
    }

    const phoneNumber = WalletRegistration.intlTelInput.getNumber();
    sendOtp(phoneNumber, reCAPTCHAToken, showNextPage, showErrorMessage);
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
    "#g-recaptcha-response-2"
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
  const button = document.getElementById("resendOtpButton");

  button.addEventListener("click", function (event) {
    resendOtp(event);
  });
});

async function resendOtp() {
  const passcodeSectionEl = document.querySelector("[data-section='passcode']");
  const errorNotificationEl = document.querySelector(
    "[data-section-error='passcode']"
  );
  const successNotificationEl = document.querySelector(
    "[data-section-success='passcode']"
  );
  const buttonEls = passcodeSectionEl.querySelectorAll("[data-button]");
  const reCAPTCHATokenEl = passcodeSectionEl.querySelector(
    "#g-recaptcha-response-2"
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
        "New OTP sent",
        "You will receive a new one-time passcode",
        true
      );
      enableButtons(buttonEls);
    }

    sendOtp(phoneNumber, reCAPTCHAToken, showSuccessMessage, showErrorMessage);
    grecaptcha.reset(1);
  }
}

function resetNumberInputError(buttonEls) {
  if (
    WalletRegistration.contactInfoErrorEl &&
    WalletRegistration.contactInfoErrorEl.style.display !== "none"
  ) {
    toggleErrorNotification(
      WalletRegistration.contactInfoErrorEl,
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
  WalletRegistration.contactInfoErrorEl = document.querySelector(
    "[data-section-error='contactInfo']"
  );
  WalletRegistration.privacyPolicyLink = getPrivacyPolicyLink();
};
