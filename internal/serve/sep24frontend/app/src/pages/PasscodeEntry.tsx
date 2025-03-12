import { FC, useState } from "react";

// Passcode Entry Component
export const PasscodeEntry: FC = () => {
  const [passcode, setPasscode] = useState("");
  const [dob, setDob] = useState("");

  const handleContinue = () => {
    // Handle verification success logic here
    alert("Verification successful!");
  };

  const handleResendOTP = () => {
    alert("New OTP sent!");
  };

  return (
    <div className="verification-container">
      <h1>Enter passcode</h1>
      <p>
        If you are pre-approved, you will receive a one-time passcode, enter it
        below to continue.
      </p>

      <div className="security-note">
        Do not share your OTP or verification data with anyone. People who ask
        for this information could be trying to access your account.
      </div>

      <div className="form-group">
        <label htmlFor="passcode-input">Passcode</label>
        <input
          id="passcode-input"
          type="text"
          value={passcode}
          onChange={(e) => setPasscode(e.target.value)}
          required
        />
      </div>

      <div className="form-group">
        <label htmlFor="dob-input">Date of birth</label>
        <input
          id="dob-input"
          type="text"
          value={dob}
          onChange={(e) => setDob(e.target.value)}
          placeholder="mm/dd/yyyy"
          required
        />
      </div>

      <div className="button-group">
        <button
          className="primary-button"
          onClick={handleContinue}
          disabled={!passcode || !dob}
        >
          Continue
        </button>

        <button className="secondary-button" onClick={handleResendOTP}>
          Resend OTP
        </button>
      </div>

      <div className="privacy-footer">
        Your data is processed by Marwen ORG in accordance with their{" "}
        <a href="#">Privacy Policy</a>.
      </div>
    </div>
  );
};
