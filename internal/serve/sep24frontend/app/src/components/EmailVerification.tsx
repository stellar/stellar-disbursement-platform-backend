import { FC, useState } from "react";
import { useNavigate } from "react-router-dom";

// Email Verification Component
export const EmailVerification: FC = () => {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (email) {
      navigate("/enter-passcode");
    }
  };

  return (
    <div className="verification-container">
      <h1>Enter your email address to get verified</h1>
      <p>
        Enter your email address below. If you are pre-approved, you will
        receive a one-time passcode.
      </p>

      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label htmlFor="email-input">Email address</label>
          <input
            id="email-input"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="Enter your email"
            required
          />
        </div>

        <button type="submit" className="primary-button" disabled={!email}>
          Submit
        </button>
      </form>
    </div>
  );
};
