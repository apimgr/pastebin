const { DataTypes } = require('sequelize');
const mongoose = require('mongoose');
const bcrypt = require('bcryptjs');
const dbManager = require('../config/database');

let User;

if (dbManager.getType() === 'mongodb') {
  const userSchema = new mongoose.Schema({
    username: {
      type: String,
      required: true,
      unique: true,
      trim: true,
      minlength: 3,
      maxlength: 30
    },
    email: {
      type: String,
      required: true,
      unique: true,
      lowercase: true,
      trim: true
    },
    password: {
      type: String,
      required: true,
      minlength: 6
    },
    tokens: [{
      name: {
        type: String,
        default: 'Default Token'
      },
      token: {
        type: String,
        required: true
      },
      createdAt: {
        type: Date,
        default: Date.now
      }
    }]
  }, {
    timestamps: true
  });

  userSchema.pre('save', async function(next) {
    if (!this.isModified('password')) return next();
    this.password = await bcrypt.hash(this.password, 12);
    next();
  });

  userSchema.methods.comparePassword = async function(candidatePassword) {
    return await bcrypt.compare(candidatePassword, this.password);
  };

  User = mongoose.model('User', userSchema);
} else {
  const initUserModel = (sequelize) => {
    User = sequelize.define('User', {
      id: {
        type: DataTypes.UUID,
        defaultValue: DataTypes.UUIDV4,
        primaryKey: true
      },
      username: {
        type: DataTypes.STRING(30),
        allowNull: false,
        unique: true,
        validate: {
          len: [3, 30],
          notEmpty: true
        }
      },
      email: {
        type: DataTypes.STRING,
        allowNull: false,
        unique: true,
        validate: {
          isEmail: true,
          notEmpty: true
        }
      },
      password: {
        type: DataTypes.STRING,
        allowNull: false,
        validate: {
          len: [6, 255],
          notEmpty: true
        }
      }
    }, {
      hooks: {
        beforeCreate: async (user) => {
          if (user.password) {
            user.password = await bcrypt.hash(user.password, 12);
          }
        },
        beforeUpdate: async (user) => {
          if (user.changed('password')) {
            user.password = await bcrypt.hash(user.password, 12);
          }
        }
      },
      timestamps: true
    });

    User.prototype.comparePassword = async function(candidatePassword) {
      return await bcrypt.compare(candidatePassword, this.password);
    };

    return User;
  };

  User = { initUserModel };
}

module.exports = User;