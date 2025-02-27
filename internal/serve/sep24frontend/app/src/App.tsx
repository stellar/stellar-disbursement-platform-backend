import { FC, useState } from 'react'
import { BrowserRouter, Routes, Route, useNavigate } from 'react-router-dom'
import './App.css'

// Types
type VerificationMethod = 'email' | 'phone' | null;

// Select Verification Method Component
const SelectVerificationMethod: FC = () => {
    const navigate = useNavigate();
    const [selectedMethod, setSelectedMethod] = useState<VerificationMethod>(null);

    const handleContinue = () => {
        if (selectedMethod === 'email') {
            navigate('/verify/email');
        } else if (selectedMethod === 'phone') {
            navigate('/verify/phone');
        }
    };

    return (
        <div className="verification-container">
            <h1>Select Verification Method</h1>
            <p>Please choose how you would like to receive your verification code.</p>

            <div className="alert-warning">
                ⚠️ Attention, it needs to match the form of contact where you received your invitation to receive this payment.
            </div>

            <div className="verification-options">
                <label className="option-label">
                    <input
                        type="radio"
                        name="verification-method"
                        value="phone"
                        checked={selectedMethod === 'phone'}
                        onChange={() => setSelectedMethod('phone')}
                    />
                    <span>Phone Number</span>
                </label>

                <label className="option-label">
                    <input
                        type="radio"
                        name="verification-method"
                        value="email"
                        checked={selectedMethod === 'email'}
                        onChange={() => setSelectedMethod('email')}
                    />
                    <span>Email</span>
                </label>
            </div>

            <button
                className="primary-button"
                onClick={handleContinue}
                disabled={!selectedMethod}
            >
                Continue
            </button>
        </div>
    );
};

// Email Verification Component
const EmailVerification: FC = () => {
    const navigate = useNavigate();
    const [email, setEmail] = useState('');

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        if (email) {
            navigate('/enter-passcode');
        }
    };

    return (
        <div className="verification-container">
            <h1>Enter your email address to get verified</h1>
            <p>Enter your email address below. If you are pre-approved, you will receive a one-time passcode.</p>

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

                <button
                    type="submit"
                    className="primary-button"
                    disabled={!email}
                >
                    Submit
                </button>
            </form>
        </div>
    );
};

// Phone Verification Component
const PhoneVerification: FC = () => {
    const navigate = useNavigate();
    const [phoneNumber, setPhoneNumber] = useState('');
    const [countryCode] = useState('+1'); // Using only the state value, not the setter

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        if (phoneNumber) {
            navigate('/enter-passcode');
        }
    };

    return (
        <div className="verification-container">
            <h1>Enter your phone number to get verified</h1>
            <p>Enter your phone number below. If you are pre-approved, you will receive a one-time passcode.</p>

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

// Passcode Entry Component
const PasscodeEntry: FC = () => {
    const [passcode, setPasscode] = useState('');
    const [dob, setDob] = useState('');

    const handleContinue = () => {
        // Handle verification success logic here
        alert('Verification successful!');
    };

    const handleResendOTP = () => {
        alert('New OTP sent!');
    };

    return (
        <div className="verification-container">
            <h1>Enter passcode</h1>
            <p>If you are pre-approved, you will receive a one-time passcode, enter it below to continue.</p>

            <div className="security-note">
                Do not share your OTP or verification data with anyone. People who ask for this information could be trying to access your account.
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

                <button
                    className="secondary-button"
                    onClick={handleResendOTP}
                >
                    Resend OTP
                </button>
            </div>

            <div className="privacy-footer">
                Your data is processed by Marwen ORG in accordance with their <a href="#">Privacy Policy</a>.
            </div>
        </div>
    );
};

// Main App Component
const App: FC = () => {
    return (
        <div className="sep24-registration">
            <BrowserRouter basename="/wallet-registration">
                <Routes>
                    <Route path="/" element={<SelectVerificationMethod />} />
                    <Route path="/start" element={<SelectVerificationMethod />} />
                    <Route path="/verify-email" element={<EmailVerification />} />
                    <Route path="/verify-phone" element={<PhoneVerification />} />
                    <Route path="/enter-passcode" element={<PasscodeEntry />} />
                    {/* Add a catch-all route that redirects to the start page */}
                    <Route path="*" element={<SelectVerificationMethod />} />
                </Routes>
            </BrowserRouter>
        </div>
    );
};

export default App;