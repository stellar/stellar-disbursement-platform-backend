import { FC, useState } from "react";
import { useNavigate } from "react-router-dom";

// Phone Verification Component
export const PhoneVerification: FC = () => {
  const navigate = useNavigate();
  const [phoneNumber, setPhoneNumber] = useState("");
  const [countryCode] = useState("+1"); // Using only the state value, not the setter

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (phoneNumber) {
      navigate("/enter-passcode");
    }
  };

  return (
    <div className="verification-container">
      <h1>Enter your phone number to get verified</h1>
      <p>
        Enter your phone number below. If you are pre-approved, you will receive
        a one-time passcode.
      </p>

      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label htmlFor="phone-input">Phone number</label>
          <div className="phone-input-container">
            <div className="country-code-selector">
              <div className="flag">🇨🇦</div>
              <span>{countryCode}</span>
            </div>
            <input
              id="phone-input"
              type="tel"
              value={phoneNumber}
              onChange={(e) => setPhoneNumber(e.target.value)}
              placeholder="(506) 234-5678"
              required
            />
          </div>
        </div>

        <button
          type="submit"
          className="primary-button"
          disabled={!phoneNumber}
        >
          Submit
        </button>
      </form>
    </div>
  );
};
